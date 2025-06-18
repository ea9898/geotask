package run

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ea9898/geotask/geo"
	"github.com/ea9898/geotask/geo/cache"
	cservice "github.com/ea9898/geotask/module/courier/service"
	cstorage "github.com/ea9898/geotask/module/courier/storage"
	"github.com/ea9898/geotask/module/courierfacade/controller"
	cfservice "github.com/ea9898/geotask/module/courierfacade/service"
	oservice "github.com/ea9898/geotask/module/order/service"
	ostorage "github.com/ea9898/geotask/module/order/storage"
	"github.com/ea9898/geotask/router"
	"github.com/ea9898/geotask/server"
	"github.com/ea9898/geotask/workers/order"
	"github.com/gin-gonic/gin"
)

type App struct {
}

func NewApp() *App {
	return &App{}
}

func (a *App) Run() error {
	// получение хоста и порта redis
	host := os.Getenv("REDIS_HOST")
	port := os.Getenv("REDIS_PORT")

	// инициализация клиента redis
	rclient := cache.NewRedisClient(host, port)

	// инициализация контекста с таймаутом
	_, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// проверка доступности redis
	_, err := rclient.Ping().Result()
	if err != nil {
		return err
	}

	// инициализация разрешенной зоны
	allowedZone := geo.NewAllowedZone()
	// инициализация запрещенных зон
	disAllowedZones := []geo.PolygonChecker{geo.NewDisAllowedZone1(), geo.NewDisAllowedZone2()}

	// инициализация хранилища заказов
	orderStorage := ostorage.NewOrderStorage(rclient)
	// инициализация сервиса заказов
	orderService := oservice.NewOrderService(orderStorage, allowedZone, disAllowedZones)

	orderGenerator := order.NewOrderGenerator(orderService)
	orderGenerator.Run()

	oldOrderCleaner := order.NewOrderCleaner(orderService)
	oldOrderCleaner.Run()

	// инициализация хранилища курьеров
	courierStorage := cstorage.NewCourierStorage(rclient)
	// инициализация сервиса курьеров
	courierSevice := cservice.NewCourierService(courierStorage, allowedZone, disAllowedZones)

	// инициализация фасада сервиса курьеров
	courierFacade := cfservice.NewCourierFacade(courierSevice, orderService)

	// инициализация контроллера курьеров
	courierController := controller.NewCourierController(courierFacade)

	// инициализация роутера
	routes := router.NewRouter(courierController)
	// инициализация сервера
	r := server.NewHTTPServer()
	// инициализация группы роутов
	api := r.Group("/api")
	// инициализация роутов
	routes.CourierAPI(api)

	mainRoute := r.Group("/")

	routes.Swagger(mainRoute)
	// инициализация статических файлов
	r.NoRoute(gin.WrapH(http.FileServer(http.Dir("public"))))

	// запуск сервера
	//serverPort := os.Getenv("SERVER_PORT")

	if os.Getenv("ENV") == "prod" {
		certFile := "/app/certs/cert.pem"
		keyFile := "/app/certs/private.pem"
		return r.RunTLS(":443", certFile, keyFile)
	}

	return r.Run()
}
