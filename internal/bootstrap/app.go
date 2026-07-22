package bootstrap

import "go.uber.org/fx"

func NewApp() *fx.App {
	return fx.New(
		CommonModules(),
		HandlerModules(),
		InvokeModules(),
	)
}
