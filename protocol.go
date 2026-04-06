package main

// ProtocolVersion is advertised over mDNS TXT, HTTP headers, and discovery JSON
// for forward-compatible mobile and desktop clients.
const ProtocolVersion = "2"

const (
	FileTransferPort = 34567
	BroadcastPort    = 9999
)

const (
	HeaderPIN             = "X-PIN"
	HeaderFileSize        = "X-File-Size"
	HeaderLocalBeamVer    = "X-LocalBeam-Version"
)
