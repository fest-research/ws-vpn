/*
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 * Author: Lukasz Zajaczkowski <zreigz@gmail.com>
 *
 */
package vpn

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/songgao/water"
)

var invalidAddr = errors.New("Invalid device ip address")

var tun_peer net.IP

func newTun(name string) (iface *water.Interface, err error) {

	iface, err = water.NewTUN(name)
	if err != nil {
		return nil, err
	}
	logger.Info("interface %v created", iface.Name())

	sargs := fmt.Sprintf("link set dev %s up mtu %d qlen 100", iface.Name(), MTU)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	logger.Info("ip %s", sargs)
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	return iface, nil
}

func setTunIP(iface *water.Interface, ip net.IP, subnet *net.IPNet) (err error) {
	ip = ip.To4()
	logger.Debug("IP address ", ip)
	if ip[3]%2 == 0 {
		return invalidAddr
	}

	peer := net.IP(make([]byte, 4))
	copy([]byte(peer), []byte(ip))
	peer[3]++
	tun_peer = peer

	sargs := fmt.Sprintf("addr add dev %s local %s peer %s", iface.Name(), ip, peer)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	logger.Info("ip ", sargs)
	err = cmd.Run()
	if err != nil {
		return err
	}

	sargs = fmt.Sprintf("route add %s via %s dev %s", subnet, peer, iface.Name())
	args = strings.Split(sargs, " ")
	cmd = exec.Command("ip", args...)
	logger.Info("ip ", sargs)
	err = cmd.Run()
	return err
}

// return net gateway (default route) and nic
func getNetGateway() (gw, dev string, err error) {

	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", "", err
	}

	defer file.Close()
	rd := bufio.NewReader(file)

	s2byte := func(s string) byte {
		b, _ := strconv.ParseUint(s, 16, 8)
		return byte(b)
	}

	for {
		line, isPrefix, err := rd.ReadLine()

		if err != nil {
			logger.Error(err.Error())
			return "", "", err
		}
		if isPrefix {
			return "", "", errors.New("Line Too Long!")
		}
		buf := bytes.NewBuffer(line)
		scanner := bufio.NewScanner(buf)
		scanner.Split(bufio.ScanWords)
		tokens := make([]string, 0, 8)

		for scanner.Scan() {
			tokens = append(tokens, scanner.Text())
		}

		iface := tokens[0]
		dest := tokens[1]
		gw := tokens[2]
		mask := tokens[7]

		if bytes.Equal([]byte(dest), []byte("00000000")) &&
			bytes.Equal([]byte(mask), []byte("00000000")) {
			a := s2byte(gw[6:8])
			b := s2byte(gw[4:6])
			c := s2byte(gw[2:4])
			d := s2byte(gw[0:2])

			ip := net.IPv4(a, b, c, d)

			return ip.String(), iface, nil
		}

	}
	return "", "", errors.New("No default gateway found")
}

// add route
func addRoute(dest, nextHop, iface string) {

	scmd := fmt.Sprintf("ip -4 r a %s via %s dev %s", dest, nextHop, iface)
	cmd := exec.Command("bash", "-c", scmd)
	logger.Info(scmd)
	err := cmd.Run()

	if err != nil {
		logger.Warning(err.Error())
	}

}

// delete route
func delRoute(dest string) {
	sargs := fmt.Sprintf("-4 route del %s", dest)
	args := strings.Split(sargs, " ")
	cmd := exec.Command("ip", args...)
	logger.Info("ip %s", sargs)
	err := cmd.Run()

	if err != nil {
		logger.Warning(err.Error())
	}
}

// redirect default gateway
func redirectGateway(iface, gw string) error {
	subnets := []string{"0.0.0.0/1", "128.0.0.0/1"}
	logger.Info("Redirecting Gateway")
	for _, subnet := range subnets {
		sargs := fmt.Sprintf("-4 route add %s via %s dev %s", subnet, gw, iface)
		args := strings.Split(sargs, " ")
		cmd := exec.Command("ip", args...)
		logger.Info("ip %s", sargs)
		err := cmd.Run()

		if err != nil {
			return err
		}
	}
	return nil
}
