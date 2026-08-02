package config

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-playground/mold/v4/modifiers"
	"github.com/go-playground/validator/v10"

	"github.com/Tech-S1/kiosk-display/internal/authpass"

	"gopkg.in/yaml.v3"
)

const displayLoopback = "127.0.0.1"

var (
	C    Config
	Path string

	conform  = modifiers.New()
	validate = validator.New()
)

type Chrome struct {
	Path    string        `yaml:"path" mod:"trim" validate:"required"`
	Display string        `yaml:"display" mod:"trim" validate:"required"`
	Width   int           `yaml:"width" validate:"gte=1"`
	Height  int           `yaml:"height" validate:"gte=1"`
	Restart time.Duration `yaml:"restart" validate:"gt=0"`
}

type Display struct {
	Port  int        `yaml:"port" validate:"gte=1,lte=65535"`
	Sleep [][]string `yaml:"sleep" validate:"dive,min=1"`
	Wake  [][]string `yaml:"wake" validate:"dive,min=1"`
}

type Manager struct {
	Bind         string `yaml:"bind" mod:"trim,default=0.0.0.0"`
	Host         string `yaml:"host" mod:"trim,default=127.0.0.1"`
	Port         int    `yaml:"port" validate:"gte=1,lte=65535"`
	PasswordHash string `yaml:"password_hash" mod:"trim" validate:"required"`
}

type Link struct {
	Label string `json:"label" yaml:"label" mod:"trim" validate:"required"`
	URL   string `json:"url" yaml:"url" mod:"trim" validate:"required,http_url"`
}

type Config struct {
	Links               []Link   `yaml:"links" json:"-" validate:"dive"`
	AllowEditLinks      bool     `yaml:"allow_edit_links" json:"allow_edit_links"`
	AllowedHosts        []string `yaml:"allowed_hosts" json:"allowed_hosts" mod:"dive,trim" validate:"dive,hostname_rfc1123|ip"`
	AutoAllowLinkHosts  bool     `yaml:"auto_allow_link_hosts" json:"auto_allow_link_hosts"`
	AutoAllowSubdomains bool     `yaml:"auto_allow_subdomains" json:"auto_allow_subdomains"`
	Chrome              Chrome   `yaml:"chrome" json:"-"`
	Display             Display  `yaml:"display" json:"-"`
	Manager             Manager  `yaml:"manager" json:"-"`
}

func (c Config) DisplayHost() string {
	return displayLoopback
}

func (c Config) DisplayListenAddr() string {
	return net.JoinHostPort(displayLoopback, strconv.Itoa(c.Display.Port))
}

func (c Config) DisplayAPIBase() string {
	return "https://" + net.JoinHostPort(displayLoopback, strconv.Itoa(c.Display.Port))
}

func (c Config) DisplayURL() string {
	return c.DisplayAPIBase() + "/"
}

func (c Config) ManagerListenAddr() string {
	return net.JoinHostPort(c.Manager.Bind, strconv.Itoa(c.Manager.Port))
}

func (c Config) PasswordHash() string {
	return c.Manager.PasswordHash
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "kiosk-display", "config.yaml")
	}
	return filepath.Join(home, ".config", "kiosk-display", "config.yaml")
}

func Load() error {
	hashPW := flag.String("hash-password", "", "print argon2id hash for the given password and exit")
	path := flag.String("config", DefaultPath(), "path to config.yaml")
	flag.Parse()

	if *hashPW != "" {
		h, err := authpass.Hash(*hashPW)
		if err != nil {
			return err
		}
		fmt.Println(h)
		os.Exit(0)
	}

	Path = *path
	b, err := os.ReadFile(Path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, &C); err != nil {
		return err
	}
	if err := conform.Struct(context.Background(), &C); err != nil {
		return err
	}
	if err := validate.Struct(C); err != nil {
		return err
	}
	if err := authpass.Validate(C.Manager.PasswordHash); err != nil {
		return fmt.Errorf("manager.password_hash: %w", err)
	}
	_ = os.Chmod(Path, 0o600)
	return nil
}
