package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime/debug"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

var commit = "?"
var timestamp = "?"

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			commit = setting.Value[:8]
		case "vcs.time":
			timestamp = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if modified {
		commit += " (modified)"
	}
}

func check(context string, err error) {
	if err != nil {
		log.Fatalf("%v: %v", context, err)
	}
}

func getAPIPassword() string {
	apiPassword := os.Getenv("WALLETD_API_PASSWORD")
	if apiPassword != "" {
		fmt.Println("env: Using WALLETD_API_PASSWORD environment variable")
	} else {
		fmt.Print("Enter API password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		check("Could not read API password:", err)
		apiPassword = string(pw)
	}
	return apiPassword
}

func main() {
	log.SetFlags(0)
	gatewayAddr := flag.String("addr", ":9981", "p2p address to listen on")
	apiAddr := flag.String("http", "localhost:9980", "address to serve API on")
	dir := flag.String("dir", ".", "directory to store node state in")
	network := flag.String("network", "mainnet", "network to connect to")
	upnp := flag.Bool("upnp", true, "attempt to forward ports and discover IP with UPnP")
	flag.Parse()

	log.Println("walletd v0.1.0")
	if flag.Arg(0) == "version" {
		log.Println("Commit Hash:", commit)
		log.Println("Commit Date:", timestamp)
		return
	}

	apiPassword := getAPIPassword()
	l, err := net.Listen("tcp", *apiAddr)
	if err != nil {
		log.Fatal(err)
	}

	// configure console logging note: this is configured before anything else
	// to have consistent logging. File logging will be added after the cli
	// flags and config is parsed
	consoleCfg := zap.NewProductionEncoderConfig()
	consoleCfg.TimeKey = "" // prevent duplicate timestamps
	consoleCfg.EncodeTime = zapcore.RFC3339TimeEncoder
	consoleCfg.EncodeDuration = zapcore.StringDurationEncoder
	consoleCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleCfg.StacktraceKey = ""
	consoleCfg.CallerKey = ""
	consoleEncoder := zapcore.NewConsoleEncoder(consoleCfg)

	// only log info messages to console unless stdout logging is enabled
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zap.NewAtomicLevelAt(zap.InfoLevel))
	logger := zap.New(consoleCore, zap.AddCaller())
	defer logger.Sync()
	// redirect stdlib log to zap
	zap.RedirectStdLog(logger.Named("stdlib"))

	if err := os.MkdirAll(*dir, 0700); err != nil {
		log.Fatal(err)
	}

	n, err := newNode(*gatewayAddr, *dir, *network, *upnp, logger)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("p2p: Listening on", n.s.Addr())
	stop := n.Start()
	log.Println("api: Listening on", l.Addr())
	go startWeb(l, n, apiPassword)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	log.Println("Shutting down...")
	stop()
}
