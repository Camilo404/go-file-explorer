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
		api.With(authMiddleware.RequireAuth).Get("/search", searchHandler.Search)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("admin")).Get("/audit", auditHandler.List)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/jobs/operations", jobsHandler.CreateOperationJob)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}", jobsHandler.GetJob)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Get("/jobs/{job_id}/items", jobsHandler.GetJobItems)
		api.With(authMiddleware.RequireAuth, authMiddleware.RequireRoles("editor", "admin")).Post("/directories", directoryHandler.Create)
	})

	return r
}
