package store

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// IcecastStagingPath is where rivapi writes the generated icecast.xml before
// installing it to /etc/icecast2/icecast.xml via sudo install.
const IcecastStagingPath = "/home/rd/etc/icecast/icecast.xml"

// IcecastDestPath is the system path icecast2 reads. Written by sudo install.
const IcecastDestPath = "/etc/icecast2/icecast.xml"

var icecastTmpl = template.Must(template.New("icecast").Parse(`<?xml version="1.0"?>
<icecast>
    <location>{{.Icecast.Location}}</location>
    <admin>{{.Icecast.AdminEmail}}</admin>

    <limits>
        <clients>{{.Icecast.MaxClients}}</clients>
        <sources>{{len .Streams}}</sources>
        <queue-size>524288</queue-size>
        <client-timeout>30</client-timeout>
        <header-timeout>15</header-timeout>
        <source-timeout>10</source-timeout>
        <burst-on-connect>1</burst-on-connect>
        <burst-size>{{.Icecast.BurstSize}}</burst-size>
    </limits>

    <authentication>
        <source-password>{{.Icecast.SourcePassword}}</source-password>
        <relay-password>{{.Icecast.RelayPassword}}</relay-password>
        <admin-user>{{.Icecast.AdminUser}}</admin-user>
        <admin-password>{{.Icecast.AdminPassword}}</admin-password>
    </authentication>

    <hostname>{{.Icecast.Hostname}}</hostname>

    <listen-socket>
        <port>{{.Icecast.Port}}</port>
    </listen-socket>

    <http-headers>
        <header name="Access-Control-Allow-Origin" value="*" />
    </http-headers>

    <fileserve>1</fileserve>

    <paths>
        <basedir>/usr/share/icecast2</basedir>
        <logdir>/var/log/icecast2</logdir>
        <webroot>/usr/share/icecast2/web</webroot>
        <adminroot>/usr/share/icecast2/admin</adminroot>
        <alias source="/" destination="/status.xsl"/>
    </paths>

    <logging>
        <accesslog>access.log</accesslog>
        <errorlog>error.log</errorlog>
        <loglevel>3</loglevel>
        <logsize>10000</logsize>
    </logging>

    <security>
        <chroot>0</chroot>
    </security>
</icecast>
`))

// GenerateIcecastXML renders icecast.xml from cfg, writes the staging file,
// then installs it to IcecastDestPath with the correct ownership via
// `sudo install`. Requires the matching sudoers rule in conf/sudoers.d/rivapi.
func GenerateIcecastXML(cfg BroadcastConfig) error {
	var buf bytes.Buffer
	if err := icecastTmpl.Execute(&buf, cfg); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(IcecastStagingPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(IcecastStagingPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	out, err := exec.Command(
		"sudo", "install",
		"-o", "root", "-g", "icecast", "-m", "640",
		IcecastStagingPath, IcecastDestPath,
	).CombinedOutput()
	if err != nil {
		return wrapOutput("install icecast.xml", err, out)
	}
	return nil
}
