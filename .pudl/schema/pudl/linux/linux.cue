package linux

// Host represents a Linux machine's identity and OS-level state.
#Host: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.host"
		identity_fields: ["hostname"]
		tracked_fields: ["os", "kernel", "arch"]
	}

	hostname:        string
	os: {
		id:      string
		version: string
		name:    string
	}
	kernel:          string
	arch:            string
	uptime_seconds:  int
	...
}

// Package represents an installed dpkg/apt package.
#Package: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.package"
		identity_fields: ["host", "name"]
		tracked_fields: ["version", "status"]
	}

	host:    string
	name:    string
	version: string
	status:  string
	...
}

// Service represents a systemd unit.
#Service: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.service"
		identity_fields: ["host", "unit"]
		tracked_fields: ["active", "sub"]
	}

	host:   string
	unit:   string
	active: string
	sub:    string
	...
}

// Filesystem represents a mounted filesystem.
#Filesystem: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.filesystem"
		identity_fields: ["host", "mountpoint"]
		tracked_fields: ["device", "fstype", "size_bytes", "used_bytes", "avail_bytes"]
	}

	host:        string
	device:      string
	mountpoint:  string
	fstype:      string
	size_bytes:  int
	used_bytes:  int
	avail_bytes: int
	...
}

// User represents a local user account from /etc/passwd.
#User: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.user"
		identity_fields: ["host", "name"]
		tracked_fields: ["uid", "gid", "home", "shell"]
	}

	host:  string
	name:  string
	uid:   int
	gid:   int
	home:  string
	shell: string
	...
}

// NetworkInterface represents a network interface from ip addr show.
#NetworkInterface: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.network_interface"
		identity_fields: ["host", "ifname"]
		tracked_fields: ["operstate", "addr_info"]
	}

	host:        string
	ifname:      string
	operstate?:  string
	flags?:      [...string]
	mtu?:        int
	link_type?:  string
	address?:    string
	addr_info?:  [...{
		family:     string
		local:      string
		prefixlen:  int
		...
	}]
	...
}
