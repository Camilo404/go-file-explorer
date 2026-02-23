package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/config"
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/websocket"
)

type Handlers struct {
	Auth          *handler.AuthHandler
	Directory     *handler.DirectoryHandler
	File          *handler.FileHandler
	Operations    *handler.OperationsHandler
	Search        *handler.SearchHandler
	Audit         *handler.AuditHandler
	Jobs          *handler.JobsHandler
	Docs          *handler.DocsHandler
	User          *handler.UserHandler
	Storage       *handler.StorageHandler
	Share         *handler.ShareHandler
	ChunkedUpload *handler.ChunkedUploadHandler
}

func New(
	cfg *config.Config,
	authMiddleware *middleware.AuthMiddleware,
	h Handlers,
	hub *websocket.Hub,
) http.Handler {
	r := chi.NewRouter()
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(cfg.AuthRateLimitRPM)

	r.Use(middleware.Recovery)
	r.Use(middleware.Logging)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.SecurityHeaders)
	r.Use(rateLimitMiddleware.Handler)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/openapi.yaml", h.Docs.OpenAPI)
	r.Get("/swagger", h.Docs.SwaggerUI)

	r.Route("/api/v1", func(api chi.Router) {

		// ── Streaming routes ─────────────────────────────────────────
		// These use StreamingTimeout instead of http.TimeoutHandler so
		// responses are NOT buffered in memory. This preserves Range
		// request support (HTTP 206) and keeps RAM flat for large files.
		streaming := middleware.StreamingTimeout(cfg.TransferTimeout, cfg.TransferIdleTimeout)

		api.With(streaming, authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/upload", h.File.Upload)
		api.With(streaming, authMiddleware.RequireAuth).Get("/files/download", h.File.Download)
		api.With(streaming, authMiddleware.RequireAuth).Get("/files/preview", h.File.Preview)
		api.With(streaming, authMiddleware.RequireAuth).Get("/files/thumbnail", h.File.Thumbnail)
		api.With(streaming, authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}/stream", h.Jobs.Stream)
		api.With(streaming).Get("/public/shares/{token}", h.Share.PublicDownload)

		// Chunked uploads — chunk write uses streaming; init/complete/abort are lightweight JSON.
		api.With(streaming, authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Put("/uploads/{upload_id}/chunks/{chunk_index}", h.ChunkedUpload.UploadChunk)

		// WebSocket endpoint for real-time notifications
		api.With(authMiddleware.RequireAuth).Get("/ws", func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeWs(hub, w, r)
		})

		// ── Standard routes ──────────────────────────────────────────
		// Short-lived JSON endpoints keep the strict http.TimeoutHandler
		// which is safe here because responses are small.
		api.Group(func(std chi.Router) {
			std.Use(middleware.Timeout(cfg.RequestTimeout))

			std.Route("/auth", func(auth chi.Router) {
				auth.Post("/login", h.Auth.Login)
				auth.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin")).Post("/register", h.Auth.Register)
				auth.Post("/refresh", h.Auth.Refresh)
				auth.With(authMiddleware.RequireAuth).Post("/logout", h.Auth.Logout)
				auth.With(authMiddleware.RequireAuth).Get("/me", h.Auth.Me)
				auth.With(authMiddleware.RequireAuth).Put("/change-password", h.Auth.ChangePassword)
			})

			std.Route("/users", func(users chi.Router) {
				users.Use(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin"))
				users.Get("/", h.User.List)
				users.Get("/{id}", h.User.Get)
				users.Put("/{id}", h.User.Update)
				users.Delete("/{id}", h.User.Delete)
			})

			std.With(authMiddleware.RequireAuth).Get("/files", h.Directory.List)
			std.With(authMiddleware.RequireAuth).Get("/tree", h.Directory.Tree)
			std.With(authMiddleware.RequireAuth).Get("/files/info", h.File.Info)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Put("/files/rename", h.Operations.Rename)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Put("/files/move", h.Operations.Move)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/copy", h.Operations.Copy)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/compress", h.Operations.Compress)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/decompress", h.Operations.Decompress)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/files", h.Operations.Delete)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/restore", h.Operations.Restore)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/trash", h.Operations.ListTrash)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/trash/{id}", h.Operations.PermanentDeleteTrash)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/trash", h.Operations.EmptyTrash)
			std.With(authMiddleware.RequireAuth).Get("/search", h.Search.Search)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin")).Get("/audit", h.Audit.List)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/jobs/operations", h.Jobs.CreateOperationJob)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}", h.Jobs.GetJob)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}/items", h.Jobs.GetJobItems)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/directories", h.Directory.Create)
			std.With(authMiddleware.RequireAuth).Get("/storage/stats", h.Storage.Stats)

			std.Route("/shares", func(shares chi.Router) {
				shares.Use(authMiddleware.RequireAuth)
				shares.With(authMiddleware.RequireRoles("editor", "admin")).Post("/", h.Share.Create)
				shares.Get("/", h.Share.List)
				shares.With(authMiddleware.RequireRoles("editor", "admin")).Delete("/{id}", h.Share.Revoke)
			})

			// Chunked uploads — JSON control endpoints.
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/uploads/init", h.ChunkedUpload.Init)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/uploads/{upload_id}/complete", h.ChunkedUpload.Complete)
			std.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/uploads/{upload_id}", h.ChunkedUpload.Abort)
		})
	})

	return r
}
