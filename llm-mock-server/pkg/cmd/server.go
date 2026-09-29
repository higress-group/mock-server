package cmd

import (
	"flag"
	"fmt"
	"os"

	"llm-mock-server/pkg/cmd/options"
	"llm-mock-server/pkg/log"
	"llm-mock-server/pkg/middleware"
	"llm-mock-server/pkg/provider/chat"
	"llm-mock-server/pkg/provider/embeddings"
	"llm-mock-server/pkg/provider/rerank"
	"llm-mock-server/pkg/provider/responses"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

func NewServerCommand() *cobra.Command {
	option := options.NewOption()
	cmd := &cobra.Command{
		Use:  "llm-mock-server",
		Long: `llm mock server for higress e2e test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Infof("run with option: %+v", option)
			return Run(option)
		},
	}
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	option.AddFlags(cmd.Flags())
	return cmd
}

func Run(option *options.Option) error {
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	server := gin.New()
	server.Use(middleware.CORS())
	middleware.StartLogger(server, option)

	// Set up chat completion routes
	chat.SetupRoutes(server, option.ProviderType)

	// embeddings
	// openai-compatible providers (hunyuan, etc.)
	server.POST("/v1/embeddings", embeddings.HandleEmbeddings)
	// baidu (v2 API)
	server.POST("/v2/embeddings", embeddings.HandleEmbeddings)

	// rerank (cohere v1)
	server.POST("/v1/rerank", rerank.HandleRerank)

	// responses API (openai and groq, which serves it under /openai/v1)
	server.POST("/v1/responses", responses.HandleResponses)
	server.POST("/openai/v1/responses", responses.HandleResponses)

	log.Infof("Starting server on port %d", option.ServerPort)
	return server.Run(fmt.Sprintf(":%d", option.ServerPort))
}
