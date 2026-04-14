package app

import (
	"github.com/punk-one/edge-service-http/config"
	"github.com/punk-one/edge-service-http/reliable"
)

type RouteRegistrar func(target any)

type Options struct {
	ConfigPath        string
	Config            *config.Config
	RouteRegistrars   []RouteRegistrar
	DeliveryObservers []reliable.DeliveryObserver
}
