package main

import (
	"fmt"
	"io"
	"log/syslog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bonan/dhcp6rd"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	execCommand     = exec.Command
	app             = kingpin.New("sixrd", "dhclient configuration helper for IPv6 rapid deployment (6rd)")
	startCmd        = app.Command("start", "(re)configure IPv6 connectivity")
	logDest         = app.Flag("log-dest", "log destination").PlaceHolder("syslog").Default("syslog").Enum("console", "syslog")
	sixrdIntf       = app.Flag("sixrd-interface", "sit interface to (de)configure").Default("ipv6rd").OverrideDefaultFromEnvar("SIXRD_INTERFACE").String()
	lanIntf         = app.Flag("lan-interface", "LAN interface to setup routing for").Envar("SIXRD_LAN_INTERFACE").String()
	ip              = startCmd.Flag("ip", "(newly) received WAN IP address").Required().String()
	sixrdOptions    = startCmd.Flag("options", "(newly) received 6rd options").Required().String()
	sixrdMTU        = startCmd.Flag("sixrd-mtu", "MTU for the tunnel").Default("1480").Envar("SIXRD_MTU").String()
	stopCmd         = app.Command("stop", "teardown IPv6 configuration")
	oldIP           = stopCmd.Flag("ip", "(old/current) WAN IP address").String()
	oldSixrdOptions = stopCmd.Flag("options", "(old/current) 6rd options").String()
	dhcpOpts        *dhcp6rd.Option6RD
	sixrdIP         string
	sixrdFullSubnet string
	sixrdPrefix     string
	sixrdPrefixSize int
	sixrdSubnet     string
	sixrdGateway    string
	errorLogger     io.Writer
	infoLogger      io.Writer
)

// setupLogger sets up where we log to. It needs to setup two destinations
// which need to conform to io.Writer, one for info messaging, one for
// error output
func setupLogger() {
	switch *logDest {
	case "syslog":
		l, err := syslog.New(syslog.LOG_NOTICE, "6rd")
		if err != nil {
			kingpin.Fatalf("could not setup syslog based logging, is syslog running?")
		}
		infoLogger = l
		l, err = syslog.New(syslog.LOG_NOTICE, "6rd")
		if err != nil {
			kingpin.Fatalf("could not setup syslog based logging, is syslog running?")
		}
		errorLogger = l
	default:
		infoLogger = os.Stdout
		errorLogger = os.Stderr
	}
	// Kingpin by default logs everything to Stderr so set the app.Writer to
	// the error logger
	app.Writer(errorLogger)
}

func ipCmd(args ...string) *exec.Cmd {
	cmd := execCommand("ip", args...)
	return cmd
}

// execute logs and executes the specified command
// though not strictly necessary it has the nice benefit of showing exactly
// which commands got run which helps a lot when trying to understand why
// everything's on fire
func execute(cmd *exec.Cmd) {
	fmt.Fprintf(infoLogger, "%s: info: executing: %s\n", app.Name, strings.Join(cmd.Args, " "))
	app.FatalIfError(cmd.Run(), "failed to execute: "+strings.Join(cmd.Args, " "))
}

func createInterface() {
	execute(ipCmd("tunnel", "add", *sixrdIntf, "mode", "sit", "local", *ip, "ttl", "64"))
}

func configureTunnel() {
	execute(ipCmd("tunnel", "6rd", "dev", *sixrdIntf, "6rd-prefix", sixrdPrefix))
	execute(ipCmd("addr", "add", sixrdIP, "dev", *sixrdIntf))
	execute(ipCmd("link", "set", "mtu", *sixrdMTU, "dev", *sixrdIntf))
}

func configureBlackhole() {
	if sixrdPrefixSize < 64 || *lanIntf == "" {
		execute(ipCmd("route", "add", "blackhole", sixrdFullSubnet, "metric", "1024"))
	}
}

func configureLAN() {
	execute(ipCmd("addr", "add", sixrdSubnet, "dev", *lanIntf))
}

func upTunnel() {
	execute(ipCmd("link", "set", *sixrdIntf, "up"))
}

func addDefaultRoute() {
	execute(ipCmd("route", "add", "default", "via", sixrdGateway, "dev", *sixrdIntf))
}

func destroyInterface() {
	cmd := ipCmd("tunnel", "del", *sixrdIntf)
	fmt.Fprintf(infoLogger, "%s: info: executing: %s\n", app.Name, strings.Join(cmd.Args, " "))
	err := cmd.Run()
	if err != nil {
		if exiterror, ok := err.(*exec.ExitError); ok {
			if exiterror.Sys().(interface {
				ExitStatus() int
			}).ExitStatus() != 1 {
				// Exit code of 1 means we tried to delete an interface that
				// doesn't exist, which is fine. It's likely that the system
				// was rebooted and it managed to properly cleanup before
				// shutdown.
				app.Fatalf("failed to execute: " + strings.Join(cmd.Args, " ") + ": " + err.Error())
			}
		} else {
			app.Fatalf("failed to execute: " + strings.Join(cmd.Args, " ") + ": " + err.Error())
		}
	}
}

func deconfigureLAN() {
	execute(ipCmd("addr", "del", sixrdSubnet, "dev", *lanIntf))
}

func deconfigureBlackhole() {
	if sixrdPrefixSize < 64 || *lanIntf == "" {
		execute(ipCmd("route", "del", sixrdFullSubnet, "dev", "lo"))
	}
}

func decodeDHCPOptions(opts string, ip string) {
	dhcpOpts, err := dhcp6rd.UnmarshalDhclient(opts)
	if err != nil {
		app.Fatalf("could not parse 6rd options")
	}
	subnet, err := dhcpOpts.IPNet(net.ParseIP(ip))
	if err != nil {
		app.Fatalf("could not determine 6rd subnet")
	}
	sixrdIP = subnet.IP.String() + "1/128"
	sixrdSubnet = subnet.IP.String() + "1/64"
	sixrdPrefixSize, _ = subnet.Mask.Size()
	sixrdFullSubnet = subnet.String()
	sixrdPrefix = dhcpOpts.Prefix.String() + "/" + strconv.Itoa(dhcpOpts.PrefixLen)
	sixrdGateway = "::" + dhcpOpts.Relay[0].String()
}

func main() {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case startCmd.FullCommand():
		setupLogger()
		decodeDHCPOptions(*sixrdOptions, *ip)
		createInterface()
		configureTunnel()
		configureBlackhole()
		upTunnel()
		addDefaultRoute()
		if *lanIntf != "" {
			configureLAN()
		}
	case stopCmd.FullCommand():
		setupLogger()
		destroyInterface()
		if *oldSixrdOptions == "" || *oldIP == "" {
			return
		}
		decodeDHCPOptions(*oldSixrdOptions, *oldIP)
		deconfigureBlackhole()
		if *lanIntf != "" {
			deconfigureLAN()
		}
	}
}
