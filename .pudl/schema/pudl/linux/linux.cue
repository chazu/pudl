package linux

// Host represents a Linux machine's identity and OS-level state.
#Host: {
	_pudl: {
		schema_type:    "base"
		resource_type:  "linux.host"
		identity_fields: ["hostname"]
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
