package main

// ProtocolVersion is advertised over mDNS TXT, HTTP headers, and discovery JSON.
const ProtocolVersion = "3"

const (
	FileTransferPort = 34567
	BroadcastPort    = 9999
)

const (
	HeaderLocalBeamVer = "X-LocalBeam-Version"
	HeaderFileSize     = "X-File-Size"
	HeaderUploadToken  = "X-Upload-Token"
	HeaderTransferID   = "X-Transfer-ID"
	HeaderSenderName   = "X-Sender-Name"
	HeaderSenderEmail  = "X-Sender-Email"
)

// Chunk size for AES-GCM streaming (plaintext per chunk).
const CryptoChunkSize = 64 * 1024
