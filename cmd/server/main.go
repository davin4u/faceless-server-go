package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/avatars"
	"github.com/davin4u/faceless-server-go/internal/config"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/files"
	"github.com/davin4u/faceless-server-go/internal/logger"
	"github.com/davin4u/faceless-server-go/internal/pow"
	"github.com/davin4u/faceless-server-go/internal/push"
	"github.com/davin4u/faceless-server-go/internal/routes"
	"github.com/davin4u/faceless-server-go/internal/socketio"
	"github.com/davin4u/faceless-server-go/internal/storage"
)

func main() {
	cfg := config.Load()
	logger.Init(logger.Config{Level: cfg.LogLevel, Format: cfg.LogFormat})
	slog.Info("server.starting", "port", cfg.Port, "db_type", cfg.DBType, "pid", os.Getpid())

	d, err := db.Open(cfg.DBType, cfg.DBPath, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db.open.error", "err", err)
		os.Exit(1)
	}
	defer d.Close()
	if err := db.InitSchema(context.Background(), d); err != nil {
		slog.Error("db.schema.error", "err", err)
		os.Exit(1)
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db.StartCleanupJobs(rootCtx, d)

	powSvc := pow.New(cfg.PoWDifficulty)
	go powSvc.StartGC(rootCtx.Done())

	sio := socketio.New(d, cfg.LogICE)
	if cfg.FCMCredentials != "" {
		sender, perr := push.New(context.Background(), cfg.FCMCredentials,
			func(ctx context.Context, userID string) ([]string, error) { return db.GetUserTokens(ctx, d, userID) },
			func(ctx context.Context, token string) { _ = db.DeleteToken(ctx, d, token) },
		)
		if perr != nil {
			slog.Error("push.init.error", "err", perr)
		} else {
			sio.SetPush(sender)
			slog.Info("push.enabled")
		}
	} else {
		slog.Info("push.disabled", "reason", "FCM_CREDENTIALS not set")
	}

	var fileRoutes *files.Service
	var avatarRoutes *avatars.Service
	if cfg.S3Bucket != "" {
		st, serr := storage.NewMinio(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL, cfg.S3Bucket)
		if serr != nil {
			slog.Error("files.init.error", "err", serr)
		} else {
			filesSvc := files.New(d, st, cfg.MaxFileSizeBytes, cfg.MaxStoragePerUserBytes)
			sio.SetFiles(filesSvc)
			filesSvc.StartCleanup(rootCtx)
			fileRoutes = filesSvc
			slog.Info("files.enabled", "bucket", cfg.S3Bucket,
				"max_file_mb", cfg.MaxFileSizeBytes/(1024*1024),
				"max_per_user_gb", cfg.MaxStoragePerUserBytes/(1024*1024*1024))
			avatarSvc := avatars.New(d, st, cfg.AvatarMaxBytes)
			avatarSvc.StartCleanup(rootCtx)
			avatarRoutes = avatarSvc
			slog.Info("avatars.enabled", "max_avatar_mb", cfg.AvatarMaxBytes/(1024*1024))
		}
	} else {
		slog.Info("files.disabled", "reason", "S3_BUCKET not set")
	}

	notifier := socketio.Notifier(sio)
	conns := socketio.ConnectionCounter(sio)

	r := chi.NewRouter()
	r.Use(logger.RealIP)
	r.Use(logger.SecurityHeaders)
	r.Use(logger.RequestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"*"},
	}))

	// Public auth endpoints (each with its own rate limit)
	authMux := routes.NewAuth(d, powSvc)
	r.With(httprate.LimitByIP(10, time.Minute)).Get("/api/pow/challenge", authMux.ServeHTTP)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/api/pow/challenge", authMux.ServeHTTP)
	r.With(httprate.LimitByIP(5, time.Minute)).Post("/api/register", authMux.ServeHTTP)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/api/recover", authMux.ServeHTTP)
	r.With(httprate.LimitByIP(20, time.Minute)).Post("/api/generate-name", authMux.ServeHTTP)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/api/invite/validate", authMux.ServeHTTP)

	// Authenticated contacts (mounted under /api/contacts)
	contactsMux := routes.NewContacts(d, notifier)
	r.With(auth.RequireSignatureAuth(d)).Mount("/api/contacts", contactsMux)

	if fileRoutes != nil {
		r.With(httprate.LimitByIP(30, time.Minute)).
			With(auth.RequireSignatureAuth(d)).
			Mount("/api/files", routes.NewFiles(fileRoutes))
	}

	if avatarRoutes != nil {
		r.With(httprate.LimitByIP(30, time.Minute)).
			With(auth.RequireSignatureAuth(d)).
			Mount("/api/avatar", routes.NewAvatars(avatarRoutes))
	}

	// Device token registration/removal
	r.With(auth.RequireSignatureAuth(d)).Method("POST", "/api/device-token", routes.NewDeviceToken(d))
	r.With(auth.RequireSignatureAuth(d)).Method("DELETE", "/api/device-token", routes.NewDeviceToken(d))

	// Admin (bearer-token, rate-limited)
	r.With(httprate.LimitByIP(60, time.Minute)).Get("/api/admin/stats", routes.NewAdmin(d, cfg.AdminSecret, conns).ServeHTTP)

	// Health (unlimited)
	r.Get("/health", routes.NewHealth().ServeHTTP)

	r.Mount("/socket.io/", sio.Handler())

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		slog.Info("server.listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server.error", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("server.shutdown.start")
	shutdownCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	defer sc()
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("server.shutdown.done")
}
