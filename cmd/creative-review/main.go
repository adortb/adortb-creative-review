// Package main 是 adortb-creative-review 服务入口。
// 提供基于 LLM 的广告素材自动审核：文案、图片、视频、着陆页。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adortb/adortb-creative-review/internal/api"
	"github.com/adortb/adortb-creative-review/internal/provider"
	"github.com/adortb/adortb-creative-review/internal/queue"
	"github.com/adortb/adortb-creative-review/internal/review"
	"github.com/adortb/adortb-creative-review/internal/rules"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	policy := rules.DefaultPolicy()
	prompts := rules.NewPromptLibrary(policy)

	// Provider 选择：集成方通过环境变量注入 API Key，服务本身不硬编码。
	llm := buildProvider(prompts)
	log.Info("provider initialized", slog.String("provider", llm.Name()))

	agg := review.NewAggregator(llm, policy, prompts)
	hq := queue.NewMemQueue()

	h := api.New(agg, hq, policy)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	port := getEnv("PORT", "8104")
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("creative-review server starting", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	case sig := <-quit:
		log.Info("shutting down", slog.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("creative-review server stopped")
}

// buildProvider 根据环境变量选择 LLM Provider。
// 未设置任何 Key 时降级为 Mock（供测试/开发使用）。
func buildProvider(prompts *rules.PromptLibrary) provider.LLMProvider {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return provider.NewOpenAIProvider(key, prompts)
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return provider.NewClaudeProvider(key, prompts)
	}
	slog.Warn("no LLM API key found, using mock provider")
	return provider.NewMockProvider()
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
