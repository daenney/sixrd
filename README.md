# sixrd

… is a helper that can be triggered by a [dhclient-script][dhs]
exit hook to configure IPv6 connectivity (essentially a dynamic 6to4
tunnel) for a host based on receving `OPTION_6RD` (212) over DHCPv4.

See [RFC5969][rfc5969] for the full specification.

Though most embedded routers that don't ship with completely crappy
software (so ASUS and anything based on opensource router firmware) can
handle 6rd just fine most Linux boxes cannot. I happen to prefer a Linux
box as my home router and use "real" home routers purely in a switch/AP
mode.

Getting 6rd going on Linux has caused me to pull out enough of my hairs
that I've decided to try and solve this issue once and for all. If you
find that this piece of software doesn't work for you that's a bug and
please file an issue for it.

**Table of Contents**

* [Building](#building)
* [Installation](#installation)
* [Configuration](#configuration)
  * [Configuring dhclient](#configuring-dhclient)
  * [Configuring IP forwarding](#configuring-ip-forwarding)
* [Usage](#usage)
  * [start](#start)
  * [stop](#stop)
* [FAQ](#faq)
  * [What ISP has this been tested with?](#what-isp-has-this-been-tested-with)
  * [Are just executing ip commands?](#are-just-executing-ip-commands)
  * [Why use this over some random script on the internet I found?](#why-use-this-over-some-random-script-on-the-internet-i-found)
  * [You only support Linux?](#you-only-support-linux)
  * [What's up with the /128 and the /64 things you're doing?](#whats-up-with-the-128-and-the-64-things-youre-doing)
  * [Why is it blackholing/null routing my whole subnet?](#why-is-it-blackholingnull-routing-my-whole-subnet)
  * [How do I get devices on my LAN to get an IPv6 IP?](#how-do-i-get-devices-on-my-lan-to-get-an-ipv6-ip)
  * [Not all my devices show an IPv6 address when looked up by hostname](#not-all-my-devices-show-an-ipv6-address-when-looked-up-by-hostname)
* [Credits](#credits)

## Building

In order to build `sixrd` you'll need Go (I've written it using 1.7 but
it should work with much older versions).

```
$ go get -u github.com/daenney/sixrd
$ cd $GOPATH/src/github.com/daenney/sixrd
$ env GOOS=linux GOARCH=amd64 go build -v
```

## Installation

Put the `sixrd` binary you built in the previous step anywhere in the
`$PATH` of your system (`/usr/bin` for example). On Debian systems
`dhclient-script` sets up its path as:
`export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`.

Put the [`6rd`][script] script in the `dhclient-exit-hooks.d` directory in
the right location on your filesystem. On Debian systems this is
`/etc/dhcp/dhclient-exit-hooks.d/`.

Ensure the `sixrd` binary and the `6rd` script are owned by `root:root`. The
binary needs to be executable, the script should not be (it's sourced by
`dhclient-script`).

## Configuration

### Configuring dhclient

By default on most distributions the [dhclient][dhc] configuration does not
attempt to request 6rd information. In order to do so you'll need to add
the following to [`dhclient.conf`][dhcconf]:

```text
option option-6rd code 212 = { integer 8, integer 8, integer 16, integer 16,
                               integer 16, integer 16, integer 16, integer 16,
                               integer 16, integer 16, array of ip-address };
```

Then add `option-6rd` to the list of parameters on the `request` line. It
doesn't matter where in that list it is, as long as it's in it.

### Configuring IP forwarding

If you start handing out IPv6 addresses to devices on your LAN you'll also
have to enable IP forwarding or your packets won't be making it out to the
world:

```
sysctl -w net.ipv6.conf.all.forwarding=1
```

Don't forget to set this through `/etc/sysctl.conf` or something similar so
it is enabled the next time you boot the machine:

```
net.ipv6.conf.all.forwarding=1
```

## Usage

`sixrd` has two subcommands, `start` and `stop` which respectively create
and configure the network interface(s) or deconfigures the setup and
destroys the tunnel interface.

By default it will manipulate an interface by the name of `ipv6rd` which
you can override using the `--sixrd-interface` option or by setting the
`SIXRD_INTERFACE` environment variable.

It will log any activity to syslog tagged with `6rd` and prefix
any message with `sixrd: ` and the severity level (info or error). The
dhclient script logs to the same tag and prefixes its output with
`dhclient: `. This gets all the logging in one place and allows you to
separate any 6rd related logging out into a separate file by configuring
syslog accordingly.

**NOTE**: You have to define the environment variables in `/etc/environment`.
This will make the environment variables available when you run `sixrd`
interactively on the console (don't forget to log out and back in first) and
the dhclient hook is configured to load and export any `SIXRD_` variable from
that same file (it needs to do this because `dhclient-script` sets up its own
custom environment).

For example:
```
SIXRD_LAN_INTERFACE=eth0
SIXRD_MTU=1420
SIXRD_INTERFACE=myCoolInterfacename
```

Please ensure you don't put `""` around the values you're assigning to your
variables as those will get passed through verbatim (which breaks things).

```
usage: sixrd [<flags>] <command> [<args> ...]

dhclient configuration helper for IPv6 rapid deployment (6rd)

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --log-dest=syslog              log destination
  --sixrd-interface="ipv6rd"     sit interface to (de)configure
  --lan-interface=LAN-INTERFACE  LAN interface to setup routing for

Commands:
  help [<command>...]
    Show help.

  start --ip=IP --options=OPTIONS [<flags>]
    (re)configure IPv6 connectivity

  stop [<flags>]
    teardown IPv6 configuration
```


### start

This subcommand must be supplied with at least the 6rd DHCP options as
well as the WAN IP address. These two are used to calculate the 6rd
prefix network and size as well as a subnet that can be configured for
your own use.

```
usage: sixrd start --ip=IP --options=OPTIONS [<flags>]

(re)configure IPv6 connectivity

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --log-dest=syslog              log destination
  --sixrd-interface="ipv6rd"     sit interface to (de)configure
  --lan-interface=LAN-INTERFACE  LAN interface to setup routing for
  --ip=IP                        (newly) received WAN IP address
  --options=OPTIONS              (newly) received 6rd options
  --sixrd-mtu="1480"             MTU for the tunnel
```

It will:

* create the `ipv6rd` interface
* configure a bunch of settings related to setting up the 6to4 tunnel
* calculate your IPv6 subnet and attach `subnet::1/128` to the
  `ipv6rd` interface

When supplied with the optional `--lan-interface` or `SIXRD_LAN_INTERFACE`
it will additionally hook up `subnet::1/64` to the LAN interface. You can
now configure a daemon to do route advertisements or DHCPv6 on the LAN
interface for that subnet.

### stop

This subcommand destroys the `ipv6rd` interface which stops/deconfigures
the tunnel.

```
usage: sixrd stop [<flags>]

teardown IPv6 configuration

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --log-dest=syslog              log destination
  --sixrd-interface="ipv6rd"     sit interface to (de)configure
  --lan-interface=LAN-INTERFACE  LAN interface to setup routing for
  --ip=IP                        (old/current) WAN IP address
  --options=OPTIONS              (old/current) 6rd options
```

When supplied with (in this case optional) the `--options`, `--ip` and
`--lan-interface` or `SIXRD_LAN_INTERFACE` it will also remove
the previously configured subnet from the LAN interface. The reason it
needs the old/current DHCP options and IP is that without them it can't
calculate the subnet. Though `stop` could guess by parsing `ip addr show`
output it risks accidentally deconfiguring the wrong network.

## FAQ

### What ISP has this been tested with?

Telia (Privat Fiber)

### Are just executing `ip` commands?

Yes. Right now that's the case, mostly because the few  netlink libraries
for Go can't do everything I need them to. I could dive one level deeper and
deal with using syscalls all over but that felt even worse.

Since all I need to do is execute commands, not interpret any output of the
`ip` command itself this felt safe enough.

### Why use this over some random script on the internet I found?

There are a variety of 6rd related scripts floating around the internet
promising to help with setting up the tunnel for you. Some even go so far as
to document how to configure `dhclient` and associated scripts correctly,
however most are in a pretty crappy state. These scripts can usually
handle correctly configuring an interface for you (but not tearing one
down, let alone correctly deal with configuration changes) and can mostly
only cope with specific prefix sizes. If your ISP deviates in any way
everything breaks down.

### You only support Linux?

No, well yes sort of. Right now that is the case. Mostly because all I have is
a Linux box. Testing this stuff isn't super trivial so I need to actually have
a live *BSD box to toy around with to ensure the end configuration works.

However, if anyone wants to it should be easy to support *BSD. The creation,
configuration, adding route, subnet null routing, deconfiguration and
destruction of this is all split up into separate functions. Those could be
moved into a `sixrd_linux.go` and have a `sixrd_bsd.go` providing the same
set of functions just calling out to other utilities.

### What's up with the /128 and the /64 things you're doing?

Once it has calculated your subnet, it picks a `/64` (as that's the smallest
IPv6 subnet one should assign) and picks the first IP of that subnet and
mounts it on the `ipv6rd` interface. A single IP in IPv6 gets a CIDR mask of
`128`, much like a single IP in IPv4 is a `/32`.

Because of this your host is now (externally) reachable over IPv6.

When you also supply the `SIXRD_LAN_INTERFACE` it additionally binds that same
IP but as part of the subnet, so with a `64` mask, on the LAN interface. Now
when devices on your LAN are assigned an IPv6 address from that subnet there's
an interface on your box bound to an IP within that subnet that can route
those packets. So you now have IPv6 connectivity for hosts on your LAN.

### Why is it blackholing/null routing my whole subnet?

Because it's trying to be a good netizen. It is possible the subnet you get
from your ISP is bigger than the one sixrd assigns (it always picks a `/64`).
However, if traffic then gets sent to another IP in your larger subnet your
ISP will route it to you, you will have no route for it so send it back,
they'll route it to you since it's your subnet and round and round it goes.

By blackholing the subnet (with a higher metric) we ensure that only traffic
for IPs that you actually have a route for are attempted to be forwarded and
silently discard everything else.

It will also configure a null route for you if you only use sixrd to setup the
tunnel, but not configure a LAN interface.

### How do I get devices on my LAN to get an IPv6 IP?

That's up to you. It's very common to use [radvd][radvd] to do route
advertisements for you and IPv6 capable devices on your LAN will pick up on
it, chose an IP from the subnet and go with it. However, if you're already
using [dnsmasq][dnsmasq] configure it to do the route advertisements for you
instead (the `enable-ra` option is what you're looking for).

DHCPv6 can be handled by a number of daemons but I recommend using dnsmasq's
built-in DHCP and DHCPv6 abilities. The configuration for RA, DHCP and DHCPv6
will look something like this:

```
bind-interfaces
interface=SIXRD_LAN_INTERFACE
dhcp-authoritative
dhcp-range=192.168.0.100,192.168.0.200,12h
dhcp-range=::,constructor:SIXRD_LAN_INTERFACE,ra-stateless,ra-names,64,12h
dhcp-option=option6:dns-server,[::]
enable-ra
```

You can find your `SIXRD_SUBNET` by looking at the output of `ip addr show
SIXRD_LAN_INTERFACE` (check for the `inet6` subnet with `scope global`) or
see what got logged to syslog.

The really cool thing with dnsmasq is that (if configured to do so) it will
create DNS entries for you for every host that gets a DHCP lease and for those
that get an IPv6 IP (usually, some things behave strangely).

In order to get that to work we need to do a few things which is mostly
trickery with how we declare the `dhcp-range`. Looking at it it might seem a
bit odd but what it does is relatively simple. It configures itself to do
DHCPv6 for the whole subnet `::` on `SIXRD_LAN_INTERFACE`. This might not be
what we want but paired with the `ra-stateless,ra-names` option we're
actually telling dnsmasq to let us do stateless IPv6 configuration (so the
host gets to pick its address). Dnsmasq will try and guess the autoconfigured
address (by using the DHCPv4 leases as a hint) and hand out any additional
options out over DHCPv6.

A slightly more complete example for dnsmasq:

```
port=53
domain-needed
bogus-priv
dhcp-authoritative
interface=SIXRD_LAN_INTERFACE
bind-interfaces
no-hosts
expand-hosts
no-negcache

# Pick an actual domain you own or use something within .local as that will
# not ever become an official TLD so we don't risk sending those queries to
# upstream DNS servers
domain=mylittlepony.local
local=/mylittlepony.local/

address=/thisbox.mylittlepony.local/192.168.0.1
address=/thisbox.mylittlepony.local/SIXRD_SUBNET::1

dhcp-range=192.168.0.100,192.168.0.200,12h
dhcp-range=::,constructor:SIXRD_LAN_INTERFACE,ra-stateless,ra-names,64,12h
dhcp-option=option6:dns-server,[::]
enable-ra

dhcp-host=MAC_ADDRESS,hostname,IPv4 # for static assignments
```

Note that doing it this way means you won't be able to configure any static
assignments by using DHCPv6. I much prefer this as it allows things like
the privacy extensions to do their thing. Since we have DNS anyway there's
little harm in having devices change/pick their address.

Unfortunately, it's not all perfect, which is where the next part of the
FAQ comes in.

### Not all my devices show an IPv6 address when looked up by hostname

Honestly, I'm clueless here. With the above setup some of my devices get both
an A and quad-A record. My Chromecast for example works fine. However none of
my iOS or macOS devices seem to get their IPv6 address registered in dnsmasq.

I've spent a couple of hours in the dnsmasq documentation and on different
forum posts and from all I can see this should work. If you happen to know or
are able to figure it out, please raise an issue and let me know!

## Credits

* [Bonan][bonan] for writing the [dhcp6rd][dhcp6rd] library
* Many random scripts on the internet showcasing (part) of how to get 6rd
  working

[dhc]: https://linux.die.net/man/8/dhclient
[dhcconf]: https://linux.die.net/man/5/dhclient.conf
[dhs]: https://linux.die.net/man/8/dhclient-script
[rfc5969]: https://tools.ietf.org/html/rfc5969#section-7.1.1
[script]: dhclient-exit-hooks.d/6rd
[dnsmasq]: http://www.thekelleys.org.uk/dnsmasq/doc.html
[radvd]: http://www.litech.org/radvd/
[bonan]: https://github.com/bonan
[dhcp6rd]: https://github.com/bonan/dhcp6rd
