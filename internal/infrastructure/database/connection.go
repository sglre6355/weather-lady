package database

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to the database described by databaseURL and returns a configured GORM handle.
func Open(databaseURL string) (*gorm.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("database url cannot be empty")
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "mysql":
		dsn, err := buildMySQLDSN(parsed)
		if err != nil {
			return nil, err
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres", "postgresql":
		return gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported database scheme %q", parsed.Scheme)
	}
}

func buildMySQLDSN(parsed *url.URL) (string, error) {
	username := ""
	if parsed.User != nil {
		username = parsed.User.Username()
	}

	password := ""
	if parsed.User != nil {
		if pwd, ok := parsed.User.Password(); ok {
			password = pwd
		}
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("database url missing hostname for mysql connection")
	}

	port := parsed.Port()
	if port == "" {
		port = "3306"
	}

	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if databaseName == "" {
		return "", fmt.Errorf("database url missing database name for mysql connection")
	}

	query := parsed.Query()
	if _, present := query["parseTime"]; !present {
		query.Set("parseTime", "true")
	}
	if _, present := query["loc"]; !present {
		query.Set("loc", "UTC")
	}

	address := net.JoinHostPort(hostname, port)
	auth := ""
	if username != "" {
		auth = username
		if password != "" {
			auth = fmt.Sprintf("%s:%s", auth, password)
		}
		auth += "@"
	} else if password != "" {
		return "", fmt.Errorf("database url provides password without username for mysql connection")
	}

	params := query.Encode()
	if params == "" {
		return fmt.Sprintf("%stcp(%s)/%s", auth, address, databaseName), nil
	}

	return fmt.Sprintf("%stcp(%s)/%s?%s", auth, address, databaseName, params), nil
}
