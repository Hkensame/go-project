package resourse

import (
	"fmt"
	"kenshop/goken/registry/registor"
	"kenshop/goken/registry/ways/consul"
	"kenshop/goken/server/rpcserver"
	"kenshop/goken/server/rpcserver/sinterceptors"
	inventorycontroller "kenshop/service/inventory/internal/controller"
	inventorylogic "kenshop/service/inventory/internal/logic"

	"github.com/hashicorp/consul/api"
)

func InitServer() {
	cfg := api.DefaultConfig()
	cfg.Address = fmt.Sprintf("%s:%d", Conf.Consul.Ip, Conf.Consul.Port)
	cli, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	regisor := registor.MustNewRegister(consul.MustNewConsulRegistor(cli))

	Server = rpcserver.MustNewServer(Ctx,
		rpcserver.WithServiceID(Conf.Id),
		rpcserver.WithServiceName(Conf.Name),
		rpcserver.WithUnaryInts(sinterceptors.UnaryTracingInterceptor),
		rpcserver.WithVersion(Conf.Version),
		rpcserver.WithRegistor(regisor),
		rpcserver.WithHost("192.168.199.128:0"),
	)

	InventoryServer = &inventorycontroller.InventoryServer{}
	InventoryServer.Logger = Logger
	InventoryServer.Service = &inventorylogic.InventoryService{}
	InventoryServer.Service.InventoryData = InventoryData
}
