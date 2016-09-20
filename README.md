# proxmox-runner
Ephemeral virtual machines on Proxmox VE

### Development

```bash
git clone https://github.com/deanrock/proxmox-runner
cd proxmox-runner/
go get
go build main.go dhcp.go
sudo PASSWORD=... ./main
```

### Configuration
Create virtual bridge `vmbr2` by adding:
```bash
auto vmbr2
iface vmbr2 inet static
  address 192.168.150.1
  netmask 255.255.255.0
  bridge_ports none
  bridge_stp off
  bridge_fd 0
  post-up iptables -t nat -A POSTROUTING -s '192.168.150.0/24' -o vmbr0 -j MASQUERADE
  post-down iptables -t nat -D POSTROUTING -s '192.168.150.0/24' -o vmbr0 -j MASQUERADE`
```
to `/etc/network/interfaces`. You might need to change `vmbr0` to the interface you use to connect to the internet (e.g. `eth0`).
