package options

import (
	"github.com/spf13/pflag"
)

type Option struct {
	ServerPort   uint32
	ProviderType string
}

func NewOption() *Option {
	return &Option{}
}

func (o *Option) AddFlags(flags *pflag.FlagSet) {
	flags.Uint32Var(&o.ServerPort, "server-port", 3000, "The server port binds to.")
	flags.StringVar(&o.ProviderType, "provider-type", "", "The provider type to use. If not specified, all routes will be enabled.")
}
