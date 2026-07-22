package main

import "github.com/endge-lab/service-template-go/internal/bootstrap"

// @title Endge Service Template API
// @version 1.0.20
// @description Production-ready шаблон Endge-сервиса с health/docs/middleware/bootstrap инфраструктурой и без бизнес-логики.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	bootstrap.NewApp().Run()
}
