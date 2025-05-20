package options

import (
	"github.com/spf13/pflag"
)

type Option struct {
	ServerPort uint32
}

func NewOption() *Option {
	return &Option{}
}

func (o *Option) AddFlags(flags *pflag.FlagSet) {
	flags.Uint32Var(&o.ServerPort, "server-port", 3000, "The server port binds to.")
}
