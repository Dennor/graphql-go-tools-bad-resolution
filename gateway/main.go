package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"go.uber.org/zap"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/pkg/playground"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	authorsgraph "github.com/Dennor/graphql-go-tools-bad-resolution/author/graph"
	booksgraph "github.com/Dennor/graphql-go-tools-bad-resolution/books/graph"
	moviesgraph "github.com/Dennor/graphql-go-tools-bad-resolution/movies/graph"
	http2 "github.com/wundergraph/graphql-go-tools/examples/federation/gateway/http"
)

// It's just a simple example of graphql federation gateway server, it's NOT a production ready code.
func logger() log.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return log.NewZapLogger(logger, log.DebugLevel)
}

func fallback(sc *ServiceConfig) (string, error) {
	dat, err := os.ReadFile(sc.Name + "/graph/schema.graphqls")
	if err != nil {
		return "", err
	}

	return string(dat), nil
}

func setupHandler(httpsrv *http.Server, prefix string, executor gqlgengraphql.ExecutableSchema) {
	srv := handler.NewDefaultServer(executor)

	http.Handle("/"+prefix, srv)
}

func serveSubgraphs(ctx context.Context) {
	srv := http.Server{
		Addr: ":8081",
	}
	setupHandler(&srv, "movies", moviesgraph.NewExecutableSchema(moviesgraph.Config{Resolvers: &moviesgraph.Resolver{}}))
	setupHandler(&srv, "books", booksgraph.NewExecutableSchema(booksgraph.Config{Resolvers: &booksgraph.Resolver{}}))
	setupHandler(&srv, "author", authorsgraph.NewExecutableSchema(authorsgraph.Config{Resolvers: &authorsgraph.Resolver{}}))
	go func() {
		_ = srv.ListenAndServe()
	}()
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
}

func startServer(ctx context.Context) {
	logger := logger()
	logger.Info("logger initialized")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	upgrader := &ws.DefaultHTTPUpgrader
	upgrader.Header = http.Header{}
	upgrader.Header.Add("Sec-Websocket-Protocol", "graphql-ws")

	graphqlEndpoint := "/query"
	playgroundURLPrefix := "/playground"
	playgroundURL := ""

	httpClient := http.DefaultClient

	mux := http.NewServeMux()

	datasourceWatcher := NewDatasourcePoller(httpClient, DatasourcePollerConfig{
		Services: []ServiceConfig{
			{Name: "books", URL: "http://localhost:8081/books"},
			{Name: "movies", URL: "http://localhost:8081/movies"},
			{Name: "author", URL: "http://localhost:8081/author"},
		},
		PollingInterval: 30 * time.Second,
	})

	p := playground.New(playground.Config{
		PathPrefix:                      "",
		PlaygroundPath:                  playgroundURLPrefix,
		GraphqlEndpointPath:             graphqlEndpoint,
		GraphQLSubscriptionEndpointPath: graphqlEndpoint,
	})

	handlers, err := p.Handlers()
	if err != nil {
		logger.Fatal("configure handlers", log.Error(err))
		return
	}

	for i := range handlers {
		mux.Handle(handlers[i].Path, handlers[i].Handler)
	}

	var gqlHandlerFactory HandlerFactoryFn = func(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler {
		return http2.NewGraphqlHTTPHandler(schema, engine, upgrader, logger)
	}

	gateway := NewGateway(gqlHandlerFactory, httpClient, logger)

	datasourceWatcher.Register(gateway)
	go datasourceWatcher.Run(ctx)

	gateway.Ready()

	mux.Handle("/query", gateway)

	addr := "0.0.0.0:8080"
	logger.Info("Listening",
		log.String("add", addr),
	)
	fmt.Printf("Access Playground on: http://%s%s%s\n", prettyAddr(addr), playgroundURLPrefix, playgroundURL)
	logger.Fatal("failed listening",
		log.Error(http.ListenAndServe(addr, mux)),
	)
}

func prettyAddr(addr string) string {
	return strings.Replace(addr, "0.0.0.0", "localhost", -1)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveSubgraphs(ctx)
	startServer(ctx)
}
