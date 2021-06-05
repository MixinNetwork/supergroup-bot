package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"

	"github.com/MixinNetwork/supergroup/durable"
	"github.com/MixinNetwork/supergroup/services"
)

func main() {
	service := flag.String("service", "http", "run a service")
	flag.Parse()

	database := durable.NewDatabase(context.Background())
	log.Println(*service)

	//mixin.UseApiHost(mixin.ZeromeshApiHost)
	//mixin.UseBlazeHost(mixin.ZeromeshBlazeHost)

	go func() {
		runtime.SetBlockProfileRate(1) // 开启对阻塞操作的跟踪
		_ = http.ListenAndServe("0.0.0.0:6060", nil)
	}()

	switch *service {
	case "http":
		err := StartHTTP(database)
		if err != nil {
			log.Println(err)
		}
	default:
		hub := services.NewHub(database)
		err := hub.StartService(*service)
		if err != nil {
			log.Println(err)
		}
	}
}
