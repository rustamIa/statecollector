package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"main/config"
	"main/sl"
	"net"
	"net/http"
	"time"

	res "main/internal/mainfetcher"
	"main/internal/model"

	"github.com/gorilla/mux"
)

//var fetch = res.GetResultData

type resultGetter func(context.Context, *slog.Logger, *config.CfgApp) (model.ResultSetT, model.ResultT)

// реализация через обёртку, чтобы спрятать varargs  custom ...fetcher и чужой тип
var fetch resultGetter = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (model.ResultSetT, model.ResultT) {
	return res.GetResultData(ctx, logger, cfg) // varargs нам тут не нужны
}

// HttpServer вызывает serveOnListener для возможности тестов с подменой serveOnListener
func HttpServer(parentCtx context.Context, logger *slog.Logger, cfg *config.CfgApp) error {
	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.HTTPAddr, err)
	}
	return serveOnListener(parentCtx, logger, cfg, ln)
}

func serveOnListener(parentCtx context.Context, logger *slog.Logger, cfg *config.CfgApp, ln net.Listener) error {
	router := mux.NewRouter()
	// один обработчик для "/"
	router.HandleFunc("/", makeHandleConnection(logger, cfg)).Methods(http.MethodGet)

	srv := &http.Server{
		Handler:           router,
		Addr:              cfg.HTTPAddr,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,  // защита от slowloris
		IdleTimeout:       60 * time.Second, // корректные keep-alive
		// Все входящие запросы унаследуют parentCtx:
		BaseContext: func(net.Listener) context.Context { return parentCtx }, //теперь каждый r.Context() — потомок parentCtx. Когда parentCtx отменится, текущие хендлеры увидят <-r.Context().Done() и корректно завершатся.
	}

	//вместо ListenAndServe тока контролируемо вручную - вынесено в HttpServer
	// ln, err := net.Listen("tcp", srv.Addr)
	// if err != nil {
	// 	return fmt.Errorf("listen %s: %w", srv.Addr, err)
	// }

	errc := make(chan error, 1)

	go func() {
		// запускаем сервер в отдельной горутине
		// Важно: не делать log.Fatal внутри горутины
		logger.Info("HTTP server start running at: " + cfg.HTTPAddr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) { // если сервер упал с реальной ошибкой (порт занят, паника и т.п.)
			errc <- err
		} else {
			errc <- nil

		}
	}()

	// Ждём либо отмену контекста, либо ошибку сервера
	select {
	case <-parentCtx.Done():
		// Нельзя использовать parentCtx для Shutdown: ато Shutdown сразу же увидит, что parentCtx уже отменён, и мгновенно завершит все соединения (форсировано без graceful), т.е. никакого «подождать активные запросы 5 секунд» не будет
		// если Go >= 1.21 — лучше так:
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), 5*time.Second) // теперь это новый контекст, который не отменится сразу по SIGTERM,а отменится через 5 секунд, если Shutdown не успеет завершить все запросы
		defer cancel()

		logger.Info("HTTP server start shutdown procedure")

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Info("HTTP server shutdown: %w", sl.Err(err))
			return fmt.Errorf("HTTP server shutdown: %w", err)
		}
		return <-errc // дождаться выхода Serve()

	case err := <-errc:
		// Сервер сам умер (порт занят, паника в хендлере и т.д.)
		return err
	}
}

// func HttpServer(parentCtx context.Context, logger *slog.Logger) {
// 	g.Go(func() error {
// 		//TODO: добавьте ему обработку адреса “/” на функцию handleConnection
// 		router := mux.NewRouter()

// 		router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
// 			// an example API handler
// 			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
// 		})
// 		router.HandleFunc("/", handleConnection)

// 		srv := &http.Server{
// 			Handler: router,
// 			Addr:    "127.0.0.1:8282",
// 			// Good practice: enforce timeouts for servers you create!
// 			WriteTimeout: 15 * time.Second,
// 			ReadTimeout:  15 * time.Second,
// 		}
// 		//res.PrepaireResStub()
// 		log.Fatal(srv.ListenAndServe())
// 		return nil
// 	})
// }

// хендлер: берёт контекст запроса, вызывает GetResultData и отдаёт JSON
func makeHandleConnection(logger *slog.Logger, cfg *config.CfgApp) http.HandlerFunc {
	type APIResponse struct {
		ResultSet model.ResultSetT `json:"resultSet"`
		Result    model.ResultT    `json:"result"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// общий бюджет на сбор данных в рамках запроса (опционально)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second) //Небольшой per-request таймаут (WithTimeout(r.Context(), 10s)) — чтобы не зависнуть, даже если кто-то внутри подвис.
		defer cancel()

		rs, rr := fetch(ctx, logger, cfg)

		// если клиент уже отвалился/таймаут — не пишем ответ
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		// enc.SetIndent("", "  ") // если нужен красивый вывод
		if err := enc.Encode(APIResponse{ResultSet: rs, Result: rr}); err != nil {
			http.Error(w, "encode error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
