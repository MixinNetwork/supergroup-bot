package middlewares

import (
	"net/http"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/session"
	"github.com/unrolled/render"
)

func Context(handler http.Handler, db *durable.Database, redis *durable.Redis, render *render.Render, logger *durable.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := session.WithRequest(r.Context(), r)
		ctx = session.WithDatabase(ctx, db)
		ctx = session.WithRedis(ctx, redis)
		ctx = session.WithRender(ctx, render)
		ctx = session.WithLogger(ctx, logger)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}
