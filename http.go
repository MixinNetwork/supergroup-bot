package main

import (
	"fmt"
	"net/http"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/middlewares"
	"github.com/MixinNetwork/supergroup/routes"
	"github.com/dimfeld/httptreemux"
	"github.com/gorilla/handlers"
	"github.com/unrolled/render"
)

func StartHTTP(db *durable.Database) error {
	router := httptreemux.New()
	routes.RegisterHanders(router)
	routes.RegisterRoutes(router)
	handler := middlewares.Authenticate(router)
	handler = middlewares.Constraint(handler)
	handler = middlewares.Context(
		handler,
		db,
		render.New(),
		&durable.Logger{},
		durable.GetMixinClient(),
	)
	handler = handlers.ProxyHeaders(handler)

	return http.ListenAndServe(fmt.Sprintf(":%d", 7001), handler)
}
