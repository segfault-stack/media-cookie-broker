package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/segfault-dev/media-cookie-broker/internal/broker"
	"github.com/segfault-dev/media-cookie-broker/internal/providers"
)

const (
	defaultDBPath  = "/data/broker.sqlite3"
	defaultKeyPath = "/run/secrets/master-key"
)

func main() {
	syscall.Umask(0o077)
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "generate-key":
		key := make([]byte, 32)
		_, err := brokerCryptoRead(key)
		check(err)
		fmt.Println(base64.RawStdEncoding.EncodeToString(key))
	case "rollback":
		runRollback(os.Args[2:])
	case "user":
		runUser(os.Args[2:])
	default:
		usage()
	}
}

func runRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	provider := fs.String("provider", "", "provider")
	profile := fs.String("profile", broker.DefaultProfile, "profile")
	revision := fs.Int64("revision", 0, "revision")
	fs.Parse(args)
	if !providers.ValidID(*provider) || *revision <= 0 || !broker.ValidProfileID(*profile) {
		check(fmt.Errorf("provider, valid profile, and positive revision are required"))
	}
	store := openStore(*db, *keyFile)
	defer store.Close()
	check(store.Rollback(context.Background(), *provider, *profile, *revision))
}

func runUser(args []string) {
	if len(args) < 1 {
		userUsage()
	}
	switch args[0] {
	case "add":
		runUserAdd(args[1:])
	case "list":
		runUserList(args[1:])
	case "delete":
		runUserDelete(args[1:])
	case "passwd":
		runUserPassword(args[1:])
	case "grant":
		runUserGrant(args[1:], false)
	case "revoke":
		runUserGrant(args[1:], true)
	default:
		userUsage()
	}
}

func runUserAdd(args []string) {
	username, rest := usernameArg(args)
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	role := fs.String("role", "", "publisher, reader, or diagnostics_reader")
	provider := fs.String("provider", "", "initial provider grant")
	profile := fs.String("profile", broker.DefaultProfile, "initial profile grant")
	passwordFile := fs.String("password-file", "", "file containing password; omitted generates one")
	fs.Parse(rest)
	if *role == "" || *provider == "" {
		check(fmt.Errorf("role and provider are required"))
	}
	password, generated, err := passwordOrGenerate(*passwordFile)
	check(err)
	store := openStore(*db, *keyFile)
	defer store.Close()
	check(store.CreateUser(context.Background(), username, password, *role, []broker.Scope{{Provider: *provider, Profile: *profile}}))
	if generated {
		fmt.Printf("Generated password for %s (shown once): %s\n", username, password)
	}
}

func runUserList(args []string) {
	fs := flag.NewFlagSet("user list", flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	fs.Parse(args)
	store := openStore(*db, *keyFile)
	defer store.Close()
	users, err := store.ListUsers(context.Background())
	check(err)
	for _, user := range users {
		scopes := make([]string, 0, len(user.Scopes))
		for _, scope := range user.Scopes {
			scopes = append(scopes, scope.Provider+"/"+scope.Profile)
		}
		fmt.Printf("%s\t%s\t%s\n", user.Username, user.Role, strings.Join(scopes, ","))
	}
}

func runUserDelete(args []string) {
	username, rest := usernameArg(args)
	fs := flag.NewFlagSet("user delete", flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	fs.Parse(rest)
	store := openStore(*db, *keyFile)
	defer store.Close()
	check(store.DeleteUser(context.Background(), username))
}

func runUserPassword(args []string) {
	username, rest := usernameArg(args)
	fs := flag.NewFlagSet("user passwd", flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	passwordFile := fs.String("password-file", "", "file containing password; omitted generates one")
	fs.Parse(rest)
	password, generated, err := passwordOrGenerate(*passwordFile)
	check(err)
	store := openStore(*db, *keyFile)
	defer store.Close()
	check(store.ChangePassword(context.Background(), username, password))
	if generated {
		fmt.Printf("Generated password for %s (shown once): %s\n", username, password)
	}
}

func runUserGrant(args []string, revoke bool) {
	username, rest := usernameArg(args)
	name := "user grant"
	if revoke {
		name = "user revoke"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	db, keyFile := storeFlags(fs)
	provider := fs.String("provider", "", "provider")
	profile := fs.String("profile", broker.DefaultProfile, "profile")
	fs.Parse(rest)
	if *provider == "" {
		check(fmt.Errorf("provider is required"))
	}
	store := openStore(*db, *keyFile)
	defer store.Close()
	if revoke {
		check(store.Revoke(context.Background(), username, *provider, *profile))
		return
	}
	check(store.Grant(context.Background(), username, *provider, *profile))
}

func storeFlags(fs *flag.FlagSet) (*string, *string) {
	return fs.String("db", defaultDBPath, "database path"), fs.String("key-file", defaultKeyPath, "master key file")
}

func usernameArg(args []string) (string, []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		userUsage()
	}
	return args[0], args[1:]
}

func passwordOrGenerate(path string) (string, bool, error) {
	if path != "" {
		password, err := passwordFromFile(path)
		return password, false, err
	}
	raw := make([]byte, 32)
	if _, err := brokerCryptoRead(raw); err != nil {
		return "", false, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), true, nil
}

func passwordFromFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("password-file is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	password := string(trim(raw))
	if password == "" {
		return "", fmt.Errorf("password file is empty")
	}
	return password, nil
}

func openStore(dbPath, keyPath string) *broker.Store {
	raw, err := os.ReadFile(keyPath)
	check(err)
	key, err := base64.RawStdEncoding.DecodeString(string(trim(raw)))
	check(err)
	store, err := broker.OpenStore(dbPath, key)
	check(err)
	return store
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: brokerctl {generate-key|rollback|user}")
	os.Exit(2)
}

func userUsage() {
	fmt.Fprintln(os.Stderr, "usage: brokerctl user {add|list|delete|passwd|grant|revoke}")
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func trim(value []byte) []byte {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}
