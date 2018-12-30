package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"layeh.com/gopus"

	"github.com/spec-tacles/voice/voice"

	"github.com/bwmarrin/discordgo"
)

type conn struct {
	voice     *voice.Connection
	cmd       *exec.Cmd
	sessionID string
	connected bool
	done      chan struct{}
	mu        sync.RWMutex
}

func newConn() *conn {
	return &conn{
		voice: voice.New(),
		done:  make(chan struct{}),
	}
}

func (c *conn) connect(endpoint string, identify *voice.Identify) (err error) {
	if err = c.voice.Connect(endpoint, identify); err != nil {
		return
	}

	c.done <- struct{}{}
	return
}

func (c *conn) waitUntilDone(timeout time.Duration) bool {
	select {
	case <-c.done:
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()

		return true
	case <-time.After(timeout):
		return false
	}
}

var (
	conns  = sync.Map{}
	prefix string
)

func main() {
	t := flag.String("t", "", "The bot's token")
	p := flag.String("p", "yarn", "The bot's prefix")
	flag.Parse()

	prefix = *p

	if *t == "" {
		fmt.Printf("A bot token is required. Please run: %s -t <token>\n", os.Args[0])
		os.Exit(1)
	}

	s, err := discordgo.New(fmt.Sprintf("Bot %s", *t))
	if err != nil {
		fmt.Printf("An error occurred while creating a Discord session: %v\n", err)
		os.Exit(1)
	}
	// s.LogLevel = discordgo.LogDebug

	s.AddHandler(messageCreate)
	s.AddHandler(voiceStateUpdate)
	s.AddHandler(voiceServerUpdate)

	if err := s.Open(); err != nil {
		fmt.Printf("An error occurred while attempting to open a Discord session: %v\n", err)
		os.Exit(1)
	}

	select {}
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot || m.GuildID == "" || !strings.HasPrefix(m.Content, prefix) {
		return
	}

	name, args := parseContent(strings.TrimSpace(strings.TrimPrefix(m.Content, prefix)))
	if name == "" {
		return
	}

	if err := runCommand(name, args, s, m.Message); err != nil {
		str := err.Error()
		if len(str) > 500 {
			str = str[:500]
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("An error has occurred\n```\n%s\n```\n```\n%s\n```", str, debug.Stack()[:1000]))
	}
}

func voiceStateUpdate(s *discordgo.Session, vsu *discordgo.VoiceStateUpdate) {
	val, ok := conns.Load(vsu.GuildID)
	if !ok {
		return
	}

	val.(*conn).sessionID = vsu.SessionID
}

func voiceServerUpdate(s *discordgo.Session, vsu *discordgo.VoiceServerUpdate) {
	val, ok := conns.Load(vsu.GuildID)
	if !ok {
		return
	}

	c := val.(*conn)

	err := c.connect("ws://"+vsu.Endpoint, &voice.Identify{
		ServerID:  parseUint64(vsu.GuildID),
		UserID:    parseUint64(s.State.User.ID),
		SessionID: c.sessionID,
		Token:     vsu.Token,
	})
	if err != nil {
		log.Println(err)
	}
}

func parseUint64(s string) uint64 {
	u, _ := strconv.ParseUint(s, 10, 64)
	return u
}

func runCommand(name string, args []string, s *discordgo.Session, m *discordgo.Message) (err error) {
	switch name {
	case "connect", "start":
		val, ok := conns.Load(m.GuildID)
		if ok {
			c := val.(*conn)

			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()

			if connected {
				s.ChannelMessageSend(m.ChannelID, "I've already connected to a channel.")
			} else {
				s.ChannelMessageSend(m.ChannelID, "I'm already attempting to connect to a channel.")
			}

			return
		}

		g, err := s.State.Guild(m.GuildID)
		if err != nil {
			return err
		}

		var channelID string
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				channelID = vs.ChannelID
				break
			}
		}

		if channelID == "" {
			s.ChannelMessageSend(m.ChannelID, "You must be in a voice channel to use this command.")
			return nil
		}

		if err = s.ChannelVoiceJoinManual(m.GuildID, channelID, false, false); err != nil {
			return err
		}

		c := newConn()
		conns.Store(m.GuildID, c)

		if !c.waitUntilDone(time.Second * 5) {
			s.ChannelMessageSend(m.ChannelID, "Timeout exceeded while attempting to join a channel.")
			conns.Delete(m.GuildID)
			return nil
		}

		s.ChannelMessageSend(m.ChannelID, "Connected!")

	case "play", "add":
		if len(args) < 1 {
			s.ChannelMessageSend(m.ChannelID, "You must provide a track to play. Example: `"+prefix+name+" https://listen.moe/opus`")
			return
		}

		val, ok := conns.Load(m.GuildID)
		if !ok {
			s.ChannelMessageSend(m.ChannelID, "I must be connected to a voice channel to run this command. Example: `"+prefix+"join`")
			return
		}

		c := val.(*conn)
		c.mu.RLock()
		cmd := c.cmd
		c.mu.RUnlock()

		if cmd != nil {
			func() {
				c.mu.Lock()
				defer c.mu.Unlock()

				err = c.cmd.Process.Kill()
				c.cmd = nil
			}()

			if err != nil {
				return
			}
		}

		c.mu.Lock()
		c.cmd = exec.Command(
			"ffmpeg",
			"-i", strings.Join(args, " "),
			"-f", "s16le",
			"-ar", strconv.Itoa(voice.SampleRate),
			"-ac", strconv.Itoa(voice.Channels),
			"pipe:1",
		)
		c.mu.Unlock()

		out, err := c.cmd.StdoutPipe()
		if err != nil {
			return err
		}

		opusEncoder, err := gopus.NewEncoder(voice.SampleRate, voice.Channels, gopus.Audio)
		if err != nil {
			return err
		}

		err = c.voice.SetSpeaking(true, 0)
		if err != nil {
			return err
		}

		errChan := make(chan error)

		go func() {
			buf := make([]int16, voice.FrameSize*voice.Channels)
			for {
				err = binary.Read(out, binary.LittleEndian, buf)
				if err != nil {
					errChan <- err
					return
				}

				opus, err := opusEncoder.Encode(buf, voice.FrameSize, voice.MaxBytes)
				if err != nil {
					errChan <- err
					return
				}

				if _, err = c.voice.UDP.Write(opus); err != nil {
					errChan <- err
					return
				}
			}
		}()

		go func() {
			errChan <- c.cmd.Start()
		}()

		err = <-errChan
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}

		return err

	case "leave", "disconnect", "remove":
		val, ok := conns.LoadOrStore(m.GuildID, voice.New())
		if !ok {
			s.ChannelMessageSend(m.ChannelID, "I must be connected to a voice channel to run this command. Example: `"+prefix+"join`")
			return
		}

		c := val.(*conn)
		if c.cmd != nil {
			c.cmd.Process.Kill()
		}

		conns.Delete(m.GuildID)
		c.voice.Close(1000, "")
		s.ChannelVoiceJoinManual(m.ChannelID, "", false, false)
	}

	return nil
}

func parseContent(content string) (string, []string) {
	parts := strings.Split(content, " ")

	if len(parts) == 0 {
		return "", nil
	}

	return strings.ToLower(parts[0]), parts[1:]
}
