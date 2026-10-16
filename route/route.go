package route

import (
	"database/sql"
	"hearable/backend/config"
	"hearable/backend/internal/ai"
	"hearable/backend/internal/auth"
	"hearable/backend/internal/listening"
	"hearable/backend/internal/progress"
	"hearable/backend/internal/speaking"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
	"hearable/backend/package/token"
	"net/http"

	"context"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"os"
	"os/signal"
	"syscall"

	"log/slog"

	"time"
)

const (
	accessTokenTTL  = 30 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour

	jsonBodyLimit  = 1 << 20
	audioBodyLimit = speaking.MaxAudioBytes + (1 << 20)
)

// NewRouter builds the HTTP handler with all dependencies wired.
func NewRouter(cfg *config.Config, db *sql.DB, logger *slog.Logger) http.Handler {
	jr := jsonresponder.NewDefaultJSONResponder()
	tokens := token.NewManager(cfg.JWTSecret, accessTokenTTL)
	aiClient := ai.NewClient(ai.Config{
		APIKey:          cfg.OpenAIAPIKey,
		BaseURL:         cfg.OpenAIBaseURL,
		EvalModel:       cfg.OpenAIEvalModel,
		TranscribeModel: cfg.OpenAITranscribeModel,
	}, logger)

	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(db), tokens, refreshTokenTTL), jr)
	listeningHandler := listening.NewHandler(listening.NewService(listening.NewRepository(db), aiClient), jr)
	speakingHandler := speaking.NewHandler(speaking.NewService(speaking.NewRepository(db), aiClient), jr)
	progressHandler := progress.NewHandler(progress.NewService(progress.NewRepository(db), cfg.Location()), jr)

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.ClientIP())
	r.Use(middleware.RequestLogger(logger))
	r.Use(chimiddleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = jr.WriteSimple(w, http.StatusOK, "ok")
	})

	authenticate := middleware.Authenticate(tokens, jr)
	// AI calls cost money; cap them per user.
	aiLimit := middleware.RateLimitByUser(jr, 20, time.Minute)

	r.Route("/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.MaxBodyBytes(jsonBodyLimit))
			r.Use(middleware.RateLimitByIP(jr, 10, time.Minute))
			r.Post("/auth/register", authHandler.Register)
			r.Post("/auth/login", authHandler.Login)
			r.Post("/auth/refresh", authHandler.Refresh)
		})

		r.Group(func(r chi.Router) {
			r.Use(authenticate)

			r.Group(func(r chi.Router) {
				r.Use(middleware.MaxBodyBytes(jsonBodyLimit))

				r.Post("/auth/logout", authHandler.Logout)
				r.Get("/me", authHandler.Me)
				r.Get("/me/activity", progressHandler.Activity)
				r.Get("/me/history", progressHandler.History)

				r.Get("/listening/modules", listeningHandler.ListModules)
				r.Post("/listening/sessions", listeningHandler.StartSession)
				r.Get("/listening/sessions/{sessionID}", listeningHandler.GetSession)
				r.With(aiLimit).Post("/listening/sessions/{sessionID}/answers", listeningHandler.SubmitAnswer)
				r.Post("/listening/sessions/{sessionID}/complete", listeningHandler.CompleteSession)

				r.Get("/speaking/modules", speakingHandler.ListModules)
				r.Post("/speaking/sessions", speakingHandler.StartSession)
				r.Get("/speaking/sessions/{sessionID}", speakingHandler.GetSession)
				r.Post("/speaking/sessions/{sessionID}/complete", speakingHandler.CompleteSession)
			})

			r.With(middleware.MaxBodyBytes(audioBodyLimit), aiLimit).
				Post("/speaking/sessions/{sessionID}/attempts", speakingHandler.SubmitAttempt)
		})
	})

	return r
}

func Start(cfg *config.Config, db *sql.DB, logger *slog.Logger) {
	apiServer := &http.Server{
		Handler:           NewRouter(cfg, db, logger),
		Addr:              ":" + cfg.Port,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// Speaking attempts call transcription + evaluation, each with a
		// 30s timeout and one retry.
		WriteTimeout: 150 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverCtx, serverCtxCancel := context.WithCancel(context.Background())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-sig

		logger.Info("Shutting down server..")

		shutdownCtx, shutdownCtxCancel := context.WithTimeout(serverCtx, 15*time.Second)

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				logger.Error("Graceful shut down timed out.. forcing exit.")
				os.Exit(1)
			}
			shutdownCtxCancel()
		}()

		err := apiServer.Shutdown(shutdownCtx)
		if err != nil {
			logger.Error("Error shutting down server", "error", err)
		}

		logger.Info("Server gracefully stopped")

		serverCtxCancel()
	}()

	go func() {
		logger.Info("Starting server", "port", cfg.Port)
		err := apiServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Error starting server", "error", err)
			serverCtxCancel()
		}
	}()

	<-serverCtx.Done()
}
