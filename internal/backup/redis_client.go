package backup

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// RedisClient handles Redis connections and protocol operations
type RedisClient struct {
	host       string
	port       string
	password   string
	tlsEnabled bool
	conn       net.Conn
	reader     *bufio.Reader

	// Connection retry settings
	maxRetries    int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
}

// NewRedisClient creates a new Redis client with the given configuration
func NewRedisClient(host, port, password string, tlsEnabled bool) *RedisClient {
	return &RedisClient{
		host:          host,
		port:          port,
		password:      password,
		tlsEnabled:    tlsEnabled,
		maxRetries:    5,
		retryDelay:    5 * time.Second,
		maxRetryDelay: 300 * time.Second,
	}
}

// SetRetryConfig configures the retry behavior
func (r *RedisClient) SetRetryConfig(maxRetries int, retryDelay, maxRetryDelay time.Duration) {
	r.maxRetries = maxRetries
	r.retryDelay = retryDelay
	r.maxRetryDelay = maxRetryDelay
}

// Connect establishes a connection to Redis with retry logic
func (r *RedisClient) Connect() error {
	addr := net.JoinHostPort(r.host, r.port)
	fmt.Printf("Connecting to Redis at %s...\n", addr)

	var conn net.Conn
	var err error

	// Retry connection with exponential backoff
	retryDelay := r.retryDelay
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if r.tlsEnabled {
			tlsConfig := &tls.Config{
				ServerName: r.host,
			}
			conn, err = tls.Dial("tcp", addr, tlsConfig)
		} else {
			conn, err = net.Dial("tcp", addr)
		}

		if err == nil {
			break
		}

		if attempt < r.maxRetries-1 {
			fmt.Printf("Connection attempt %d failed: %v. Retrying in %v...\n",
				attempt+1, err, retryDelay)
			time.Sleep(retryDelay)

			// Exponential backoff with max limit
			retryDelay *= 2
			if retryDelay > r.maxRetryDelay {
				retryDelay = r.maxRetryDelay
			}
		}
	}

	if err != nil {
		return fmt.Errorf("connection failed after %d attempts: %v", r.maxRetries, err)
	}

	r.conn = conn
	r.reader = bufio.NewReader(conn)
	fmt.Println("Connected to Redis successfully")
	return nil
}

// Authenticate performs Redis AUTH command if password is provided
func (r *RedisClient) Authenticate() error {
	if r.password == "" {
		return nil
	}

	authCmd := fmt.Sprintf("AUTH %s\r\n", r.password)
	if _, err := r.conn.Write([]byte(authCmd)); err != nil {
		return fmt.Errorf("failed to send AUTH command: %v", err)
	}

	// Read response
	response, err := r.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read AUTH response: %v", err)
	}

	if !strings.HasPrefix(response, "+OK") {
		return fmt.Errorf("authentication failed: %s", strings.TrimSpace(response))
	}

	fmt.Println("Authentication successful")
	return nil
}

// SendPSYNC sends PSYNC command and parses the FULLRESYNC response
func (r *RedisClient) SendPSYNC() (*ReplicationInfo, error) {
	// Send PSYNC command to force full resynchronization
	psyncCmd := "PSYNC ? -1\r\n"
	if _, err := r.conn.Write([]byte(psyncCmd)); err != nil {
		return nil, fmt.Errorf("failed to send PSYNC command: %v", err)
	}

	// Read FULLRESYNC response
	response, err := r.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read PSYNC response: %v", err)
	}

	response = strings.TrimSpace(response)
	if !strings.HasPrefix(response, "+FULLRESYNC") {
		return nil, fmt.Errorf("unexpected PSYNC response: %s", response)
	}

	// Parse FULLRESYNC response: +FULLRESYNC <replid> <offset>
	parts := strings.Fields(response)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid FULLRESYNC format: %s", response)
	}

	replID := parts[1]
	offset, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid offset in FULLRESYNC: %v", err)
	}

	fmt.Printf("Received FULLRESYNC: ID=%s, Offset=%d\n", replID, offset)
	return &ReplicationInfo{ID: replID, Offset: offset}, nil
}

// ReadRDBSnapshot reads the RDB bulk string and returns the data length
func (r *RedisClient) ReadRDBSnapshot() ([]byte, error) {
	// Read the RDB bulk string header
	// Format: $<length>\r\n<data>

	// Read the '$' character
	prefix, err := r.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read RDB prefix: %v", err)
	}
	if prefix != '$' {
		return nil, fmt.Errorf("expected '$' for RDB bulk string, got '%c'", prefix)
	}

	// Read the length
	lengthStr, err := r.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read RDB length: %v", err)
	}

	lengthStr = strings.TrimSpace(lengthStr)
	length, err := strconv.ParseInt(lengthStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid RDB length: %v", err)
	}

	fmt.Printf("Reading RDB snapshot (%d bytes)...\n", length)

	// Read RDB data
	data := make([]byte, length)
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return nil, fmt.Errorf("failed to read RDB data: %v", err)
	}

	fmt.Printf("RDB snapshot read successfully (%d bytes)\n", length)
	return data, nil
}

// ReadRESPCommand reads a RESP command from the replication stream
func (r *RedisClient) ReadRESPCommand() (string, error) {
	// Read RESP array (Redis commands are sent as arrays)
	// Format: *<count>\r\n$<len>\r\n<data>\r\n...

	firstByte, err := r.reader.ReadByte()
	if err != nil {
		return "", err
	}

	if firstByte != '*' {
		return "", fmt.Errorf("expected '*' for RESP array, got '%c'", firstByte)
	}

	// Read array count
	countStr, err := r.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	countStr = strings.TrimSpace(countStr)
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return "", fmt.Errorf("invalid array count: %v", err)
	}

	// Build the complete RESP command
	var command strings.Builder
	command.WriteString(fmt.Sprintf("*%d\r\n", count))

	// Read each element of the array
	for i := 0; i < count; i++ {
		element, err := r.readRESPBulkString()
		if err != nil {
			return "", fmt.Errorf("failed to read array element %d: %v", i, err)
		}
		command.WriteString(element)
	}

	return command.String(), nil
}

// readRESPBulkString reads a RESP bulk string
func (r *RedisClient) readRESPBulkString() (string, error) {
	// Read RESP bulk string: $<len>\r\n<data>\r\n
	prefix, err := r.reader.ReadByte()
	if err != nil {
		return "", err
	}

	if prefix != '$' {
		return "", fmt.Errorf("expected '$' for bulk string, got '%c'", prefix)
	}

	lengthStr, err := r.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	lengthStr = strings.TrimSpace(lengthStr)
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", fmt.Errorf("invalid bulk string length: %v", err)
	}

	if length == -1 {
		return "$-1\r\n", nil // NULL bulk string
	}

	// Read the data
	data := make([]byte, length+2) // +2 for \r\n
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return "", fmt.Errorf("failed to read bulk string data: %v", err)
	}

	return fmt.Sprintf("$%d\r\n%s", length, string(data)), nil
}

// IsConnected returns true if the client has an active connection
func (r *RedisClient) IsConnected() bool {
	return r.conn != nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		r.reader = nil
		fmt.Println("Redis connection closed")
		return err
	}
	return nil
}

// GetReader returns the buffered reader for low-level access
func (r *RedisClient) GetReader() *bufio.Reader {
	return r.reader
}
