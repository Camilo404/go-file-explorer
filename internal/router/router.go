package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/config"
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
)

func New(
	cfg *config.Config,
	authMiddleware *middleware.AuthMiddleware,
	authHandler *handler.AuthHandler,
	directoryHandler *handler.DirectoryHandler,
	fileHandler *handler.FileHandler,
	operationsHandler *handler.OperationsHandler,
	searchHandler *handler.SearchHandler,
	auditHandler *handler.AuditHandler,
	jobsHandler *handler.JobsHandler,
	docsHandler *handler.DocsHandler,
	userHandler *handler.UserHandler,
	storageHandler *handler.StorageHandler,
	shareHandler *handler.ShareHandler,
) http.Handler {
	r := chi.NewRouter()
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(cfg.RateLimitRPM, cfg.AuthRateLimitRPM)

	r.Use(middleware.Recovery)
	r.Use(middleware.Logging)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.SecurityHeaders)
	r.Use(rateLimitMiddleware.Handler)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/openapi.yaml", docsHandler.OpenAPI)
	r.Get("/swagger", docsHandler.SwaggerUI)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(middleware.Timeout(cfg.RequestTimeout))

		api.Route("/auth", func(auth chi.Router) {
			auth.Post("/login", authHandler.Login)
			auth.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin")).Post("/register", authHandler.Register)
			auth.Post("/refresh", authHandler.Refresh)
			auth.With(authMiddleware.RequireAuth).Post("/logout", authHandler.Logout)
			auth.With(authMiddleware.RequireAuth).Get("/me", authHandler.Me)
			auth.With(authMiddleware.RequireAuth).Put("/change-password", authHandler.ChangePassword)
		})

		api.Route("/users", func(users chi.Router) {
			users.Use(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin"))
			users.Get("/", userHandler.List)
			users.Get("/{id}", userHandler.Get)
			users.Put("/{id}", userHandler.Update)
			users.Delete("/{id}", userHandler.Delete)
		})

		api.With(authMiddleware.RequireAuth).Get("/files", directoryHandler.List)
		api.With(authMiddleware.RequireAuth).Get("/tree", directoryHandler.Tree)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/upload", fileHandler.Upload)
		api.With(authMiddleware.RequireAuth).Get("/files/download", fileHandler.Download)
		api.With(authMiddleware.RequireAuth).Get("/files/preview", fileHandler.Preview)
		api.With(authMiddleware.RequireAuth).Get("/files/thumbnail", fileHandler.Thumbnail)
		api.With(authMiddleware.RequireAuth).Get("/files/info", fileHandler.Info)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Put("/files/rename", operationsHandler.Rename)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Put("/files/move", operationsHandler.Move)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/copy", operationsHandler.Copy)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/files", operationsHandler.Delete)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/files/restore", operationsHandler.Restore)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/trash", operationsHandler.ListTrash)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/trash/{id}", operationsHandler.PermanentDeleteTrash)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Delete("/trash", operationsHandler.EmptyTrash)
		api.With(authMiddleware.RequireAuth).Get("/search", searchHandler.Search)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin")).Get("/audit", auditHandler.List)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/jobs/operations", jobsHandler.CreateOperationJob)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}", jobsHandler.GetJob)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}/items", jobsHandler.GetJobItems)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}/stream", jobsHandler.Stream)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/directories", directoryHandler.Create)
		api.With(authMiddleware.RequireAuth).Get("/storage/stats", storageHandler.Stats)

		api.Route("/shares", func(shares chi.Router) {
			shares.Use(authMiddleware.RequireAuth)
			shares.With(authMiddleware.RequireRoles("editor", "admin")).Post("/", shareHandler.Create)
			shares.Get("/", shareHandler.List)
			shares.With(authMiddleware.RequireRoles("editor", "admin")).Delete("/{id}", shareHandler.Revoke)
		})

		api.Get("/public/shares/{token}", shareHandler.PublicDownload)
	})

	return r
}
