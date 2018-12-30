package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spec-tacles/voice/voice"
	"github.com/spf13/cobra"
)

var (
	endpoint  *string
	serverID  *uint64
	userID    *uint64
	sessionID *string
	token     *string
)

var rootCmd = &cobra.Command{
	Use:   "voice",
	Short: "A voice utility for Discord.",
	Long: `Takes stdin and transmits to Discord. Piped data must be Opus audio packets with
2 channels and 48kHz sample rate.`,
	Run: func(c *cobra.Command, args []string) {
		v := voice.New()
		v.Connect(*endpoint, &voice.Identify{
			ServerID:  *serverID,
			SessionID: *sessionID,
			UserID:    *userID,
			Token:     *token,
		})

		_, err := io.Copy(v.UDP, os.Stdin)
		if err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	rootCmd.Flags().StringVar(endpoint, "endpoint", "", "voice endpoint to connect to (VOICE_SERVER_UPDATE)")
	rootCmd.Flags().Uint64Var(serverID, "server_id", 0, "guild ID of this voice connection (VOICE_STATE_UPDATE/VOICE_SERVER_UPDATE)")
	rootCmd.Flags().Uint64Var(userID, "user_id", 0, "user ID of this voice connection (VOICE_STATE_UPDATE)")
	rootCmd.Flags().StringVar(sessionID, "session_id", "", "session ID of this voice connection (VOICE_STATE_UPDATE)")
	rootCmd.Flags().StringVar(token, "token", "", "voice session token (VOICE_SERVER_UPDATE)")
	rootCmd.MarkFlagRequired("endpoint")
	rootCmd.MarkFlagRequired("server_id")
	rootCmd.MarkFlagRequired("user_id")
	rootCmd.MarkFlagRequired("session_id")
	rootCmd.MarkFlagRequired("token")
}

// Execute the CLI app
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
