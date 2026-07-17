package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	tunneldomain "shh-h/internal/domain/tunnel"
)

const socksHandshakeTimeout = 10 * time.Second

func (s *Service) acceptLoop(runtime *runtimeTunnel) error {
	for {
		listener := runtime.listenerValue()
		if listener == nil {
			return errors.New("tunnel listener is unavailable")
		}
		incoming, err := listener.Accept()
		if err != nil {
			if runtime.ctx.Err() != nil {
				return runtime.ctx.Err()
			}
			return fmt.Errorf("accept tunnel connection: %w", err)
		}
		select {
		case runtime.relays <- struct{}{}:
			runtime.track(incoming)
			runtime.wait.Add(1)
			go func() {
				defer runtime.wait.Done()
				defer func() { <-runtime.relays }()
				s.handleConnection(runtime, incoming)
			}()
		default:
			_ = incoming.Close()
		}
	}
}

func (s *Service) handleConnection(runtime *runtimeTunnel, incoming net.Conn) {
	defer runtime.untrackAndClose(incoming)
	var destinationAddress string
	if runtime.config.Kind == tunneldomain.KindDynamic {
		if err := incoming.SetDeadline(time.Now().Add(socksHandshakeTimeout)); err != nil {
			return
		}
		address, err := readSOCKS5Target(incoming)
		if err != nil {
			return
		}
		destinationAddress = address
	} else {
		destinationAddress = net.JoinHostPort(runtime.config.DestinationHost, strconv.Itoa(runtime.config.DestinationPort))
	}

	var destination net.Conn
	var err error
	if runtime.config.Kind == tunneldomain.KindRemote {
		destination, err = (&net.Dialer{Timeout: 15 * time.Second}).DialContext(runtime.ctx, "tcp", destinationAddress)
	} else {
		connection := runtime.connectionValue()
		if connection == nil {
			return
		}
		destination, err = connection.Dial("tcp", destinationAddress)
	}
	if err != nil {
		if runtime.config.Kind == tunneldomain.KindDynamic {
			_ = writeSOCKS5Reply(incoming, 0x04)
		}
		return
	}
	runtime.track(destination)
	defer runtime.untrackAndClose(destination)
	if runtime.config.Kind == tunneldomain.KindDynamic {
		if err := writeSOCKS5Reply(incoming, 0x00); err != nil {
			return
		}
		if err := incoming.SetDeadline(time.Time{}); err != nil {
			return
		}
	}
	relayConnections(incoming, destination)
}

func relayConnections(left, right net.Conn) {
	done := make(chan struct{}, 2)
	copyHalf := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		if closer, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyHalf(left, right)
	go copyHalf(right, left)
	<-done
	<-done
}

func readSOCKS5Target(connection net.Conn) (string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(connection, header); err != nil {
		return "", err
	}
	if header[0] != 0x05 || header[1] == 0 {
		return "", errors.New("invalid SOCKS5 greeting")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(connection, methods); err != nil {
		return "", err
	}
	supportsNoAuthentication := false
	for _, method := range methods {
		if method == 0x00 {
			supportsNoAuthentication = true
		}
	}
	if !supportsNoAuthentication {
		_, _ = connection.Write([]byte{0x05, 0xff})
		return "", errors.New("SOCKS5 client requires unsupported authentication")
	}
	if _, err := connection.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(connection, request); err != nil {
		return "", err
	}
	if request[0] != 0x05 || request[1] != 0x01 || request[2] != 0x00 {
		_ = writeSOCKS5Reply(connection, 0x07)
		return "", errors.New("only SOCKS5 CONNECT is supported")
	}
	var host string
	switch request[3] {
	case 0x01:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(connection, length); err != nil {
			return "", err
		}
		if length[0] == 0 {
			return "", errors.New("empty SOCKS5 domain")
		}
		address := make([]byte, int(length[0]))
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		host = string(address)
	case 0x04:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		host = net.IP(address).String()
	default:
		_ = writeSOCKS5Reply(connection, 0x08)
		return "", errors.New("unsupported SOCKS5 address type")
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(connection, portBytes); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), nil
}

func writeSOCKS5Reply(connection net.Conn, status byte) error {
	_, err := connection.Write([]byte{0x05, status, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	return err
}

func (r *runtimeTunnel) listenerValue() net.Listener {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listener
}

func (r *runtimeTunnel) track(connection net.Conn) {
	r.mu.Lock()
	r.connections[connection] = struct{}{}
	r.mu.Unlock()
}

func (r *runtimeTunnel) untrackAndClose(connection net.Conn) {
	r.mu.Lock()
	delete(r.connections, connection)
	r.mu.Unlock()
	_ = connection.Close()
}
