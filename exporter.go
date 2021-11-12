// sb8200-exporter, a Prometheus exporter for Arris SB8200 Modems
// Copyright (C) 2021  Mark Stenglein
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
package main

import (
	"crypto/tls"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

type DownstreamChannel struct {
	ChannelID           string  // Channel identifier (string)
	LockStatus          float64 // Whether the channel is locked or not (boolean)
	Modulation          string  // Type of modulation used by channel
	Frequency           string  // Frequency the channel is operating on (Hz)
	Power               float64 // Power level (dBmV)
	SNR                 float64 // SNR/MER (dB)
	CorrectedErrors     float64 // Counter, resets to 0 on modem reboot (n)
	UncorrectableErrors float64 // Counter, resets to 0 on modem reboot (n)
}

type UpstreamChannel struct {
	Channel       string  // Channel Number (string)
	ChannelID     string  // Channel ID (string)
	LockStatus    float64 // Whether the channel is locked or not (boolean)
	USChannelType string  // Upstream channel modulation
	Frequency     string  // Frequency the channel is operating on (Hz)
	Width         string  // Channel width (Hz)
	Power         float64 // Power level (dBmV)
}

type ArrisModem struct {
	Host                     string              // Hostname or network address of SB8200 modem
	ConnectivityState        float64             // Is the modem connected to upstream provider (boolean)
	Uptime                   float64             // From product info page, Uptime (Seconds)
	HardwareVersion          string              // From product info page
	SoftwareVersion          string              // From product info page
	MACAddress               string              // From product info page
	SerialNumber             string              // From product info page
	DownstreamBondedChannels []DownstreamChannel // From status page, array of channels
	UpstreamBondedChannels   []UpstreamChannel   // From status page, array of channels
}

type Exporter struct {
	Host      string // Hostname or network address of SB8200 modem
	AuthToken string // b64 encoded username:password
}

func NewExporter(host string, user string, pass string) *Exporter {
	return &Exporter{
		Host:      host,
		AuthToken: b64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, pass))),
	}
}

// Log into the web interface and return sessionID and csrf token
func (e *Exporter) Login() (sessionID *http.Cookie, csrfToken string, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/logout.html", e.Host), nil)
	if err != nil {
		return
	}
	defer req.Body.Close()
	client.Do(req)

	url := fmt.Sprintf("https://%s/cmconnectionstatus.html?login_%s", e.Host, e.AuthToken)
	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var body []byte
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		csrfToken = string(body)

		for _, cookie := range resp.Cookies() {
			// The server will set the sessionID to "" whenever it wants to
			//   force and signal the end of a session.
			if cookie.Name == "sessionId" && cookie.Value != "" {
				sessionID = cookie
				return
			}
		}

		err = errors.New("missing sessionID")
		return
	}

	if resp.StatusCode == http.StatusUnauthorized {
		err = errors.New("invalid credentials")
		return
	}

	err = errors.New("unknown error/response code")
	return
}

func ScrapeColStr(element *goquery.Selection, child int) string {
	selectString := fmt.Sprintf("td:nth-child(%d)", child)
	return element.Find(selectString).First().Text()
}

func ScrapeUnitValue(element *goquery.Selection, child int, trim string) (float64, error) {
	valStr := strings.TrimRight(ScrapeColStr(element, child), trim)
	valFloat, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0, err
	}
	return valFloat, nil
}

func ScrapeDownstreamTableRow(element *goquery.Selection) (downstreamChannel DownstreamChannel, err error) {
	// Skip first row (that shows header values)
	if ScrapeColStr(element, 1) == "Channel ID" {
		err = errors.New("skip parsing second header row")
		return
	}

	lockStatus := 0.
	if ScrapeColStr(element, 2) == "Locked" {
		lockStatus = 1.
	}

	power, err := ScrapeUnitValue(element, 5, " dBmV")
	if err != nil {
		return
	}

	snr, err := ScrapeUnitValue(element, 6, " dB")
	if err != nil {
		return
	}

	correctedErrors, err := ScrapeUnitValue(element, 7, "")
	if err != nil {
		return
	}

	uncorrectableErrors, err := ScrapeUnitValue(element, 8, "")
	if err != nil {
		return
	}

	downstreamChannel = DownstreamChannel{
		ChannelID:           ScrapeColStr(element, 1),
		LockStatus:          lockStatus,
		Modulation:          ScrapeColStr(element, 3),
		Frequency:           ScrapeColStr(element, 4),
		Power:               power,
		SNR:                 snr,
		CorrectedErrors:     correctedErrors,
		UncorrectableErrors: uncorrectableErrors,
	}
	return
}

func ScrapeDownstreamTable(element *goquery.Selection) (downstreamChannels []DownstreamChannel) {
	element.Each(func(index int, element *goquery.Selection) {
		parsedRow, err := ScrapeDownstreamTableRow(element)
		if err != nil {
			log.Debug(err)
			return
		}
		downstreamChannels = append(downstreamChannels, parsedRow)
	})
	return
}

func ScrapeUpstreamTableRow(element *goquery.Selection) (upstreamChannel UpstreamChannel, err error) {
	// Skip first row (that shows header values)
	if firstVal := ScrapeColStr(element, 1); firstVal == "Channel" || firstVal == "" {
		err = errors.New("skip first two header row")
		return
	}

	lockStatus := 0.
	if ScrapeColStr(element, 3) == "Locked" {
		lockStatus = 1.
	}

	power, err := ScrapeUnitValue(element, 7, " dBmV")
	if err != nil {
		return
	}

	upstreamChannel = UpstreamChannel{
		Channel:       ScrapeColStr(element, 1),
		ChannelID:     ScrapeColStr(element, 2),
		LockStatus:    lockStatus,
		USChannelType: ScrapeColStr(element, 4),
		Frequency:     ScrapeColStr(element, 5),
		Width:         ScrapeColStr(element, 6),
		Power:         power,
	}
	return
}

func ScrapeUpstreamTable(element *goquery.Selection) (upstreamChannels []UpstreamChannel) {
	element.Each(func(index int, element *goquery.Selection) {
		parsedRow, err := ScrapeUpstreamTableRow(element)
		if err != nil {
			log.Debug(err)
			return
		}
		upstreamChannels = append(upstreamChannels, parsedRow)
	})
	return
}

func GetURL(url string, sessionID *http.Cookie) (document *goquery.Document, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.AddCookie(sessionID)
	defer req.Body.Close()

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	document, err = goquery.NewDocumentFromReader(resp.Body)
	return
}

// Scrape the web page for metric data
func (e *Exporter) Scrape() (modem ArrisModem, err error) {
	sessionID, csrfToken, err := e.Login()
	if err != nil {
		log.Error("Failed to fetch login tokens")
		return
	}

	url := fmt.Sprintf("https://%s/cmconnectionstatus.html?ct_%s", e.Host, csrfToken)
	document, err := GetURL(url, sessionID)
	if err != nil {
		log.Error("Failed to fetch connection status url")
		return
	}

	connectivityStateSelector := ".content > center:nth-child(2) > table:nth-child(1) > tbody:nth-child(1) > tr:nth-child(4) > td:nth-child(2)"
	connectivityState := 0.
	if document.Find(connectivityStateSelector).First().Text() == "OK" {
		connectivityState = 1.
	}

	var downstreamChannels []DownstreamChannel
	var upstreamChannels []UpstreamChannel
	document.Find("table").Each(func(i int, element *goquery.Selection) {
		switch i {
		case 1:
			downstreamChannels = ScrapeDownstreamTable(element.Find("tr"))
		case 2:
			upstreamChannels = ScrapeUpstreamTable(element.Find("tr"))
		}
	})

	url = fmt.Sprintf("https://%s/cmswinfo.html?ct_%s", e.Host, csrfToken)
	document, err = GetURL(url, sessionID)
	if err != nil {
		log.Error("Failed to fetch product information page")
		return
	}

	hwVerSelector := "table.simpleTable:nth-child(2) > tbody:nth-child(1) > tr:nth-child(3) > td:nth-child(2)"
	hwVersion := document.Find(hwVerSelector).First().Text()

	swVerSelector := "table.simpleTable:nth-child(2) > tbody:nth-child(1) > tr:nth-child(4) > td:nth-child(2)"
	swVersion := document.Find(swVerSelector).First().Text()

	macAddrSelector := "table.simpleTable:nth-child(2) > tbody:nth-child(1) > tr:nth-child(5) > td:nth-child(2)"
	macAddress := document.Find(macAddrSelector).First().Text()

	serialSelector := "table.simpleTable:nth-child(2) > tbody:nth-child(1) > tr:nth-child(6) > td:nth-child(2)"
	serial := document.Find(serialSelector).First().Text()

	uptimeSelector := "table.simpleTable:nth-child(5) > tbody:nth-child(1) > tr:nth-child(2) > td:nth-child(2)"
	// uptimeStr will look like: 40 days 05h:32m:52s.00
	uptimeStr := document.Find(uptimeSelector).First().Text()
	// parts will look like ["40" "05" "32" "52" "00"]
	uptimeParts := regexp.MustCompile(`\D+`).Split(uptimeStr, -1)
	uptime := 0.
	for i, nStr := range uptimeParts {
		var n float64
		n, err = strconv.ParseFloat(nStr, 64)
		if err != nil {
			return
		}
		switch i {
		case 0: // days
			uptime = n
		case 1: // hours
			uptime = uptime*24 + n
		case 2: // minutes
			uptime = uptime*60 + n
		case 3: // seconds
			uptime = uptime*60 + n
		} // ignore milliseconds
	}

	modem = ArrisModem{
		Host:                     e.Host,
		ConnectivityState:        connectivityState,
		Uptime:                   uptime,
		HardwareVersion:          hwVersion,
		SoftwareVersion:          swVersion,
		MACAddress:               macAddress,
		SerialNumber:             serial,
		DownstreamBondedChannels: downstreamChannels,
		UpstreamBondedChannels:   upstreamChannels,
	}
	return
}

const (
	namespace  = "sb8200"
	DOWNSTREAM = "downstream"
	UPSTREAM   = "upstream"
)

var (
	// Metrics
	upMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last data scrape successful?",
		nil, nil,
	)
	connectedMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "connected"),
		"Is the modem's connection up (connectivity state)?",
		nil, nil,
	)
	uptimeMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "uptime_seconds"),
		"Uptime",
		nil, nil,
	)
	metaMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "info"),
		"Metadata about this modem.",
		[]string{"host", "hwversion", "swversion", "mac", "serial"},
		nil,
	)
	channelLockMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "lock"),
		"Is the downstream channel locked?",
		[]string{"channel_id", "type"}, nil,
	)
	channelPowerMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "power"),
		"Power level (dBmV)",
		[]string{"channel_id", "type"}, nil,
	)
	channelSNRMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "snr"),
		"SNR/MER rate (dB)",
		[]string{"channel_id", "type"}, nil,
	)
	channelCorrectedMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "corrected_total"),
		"Corrected errors, counter resets to 0 on modem reboot",
		[]string{"channel_id", "type"}, nil,
	)
	channelUncorrectableMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "uncorrectable_total"),
		"Uncorrectable errors, counter resets to 0 on modem reboot",
		[]string{"channel_id", "type"}, nil,
	)
	channelMetaMetric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "channel", "info"),
		"Channel metadata",
		[]string{"channel_id", "modulation", "frequency", "width", "type"}, nil,
	)
)

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- upMetric
	ch <- connectedMetric
	ch <- uptimeMetric
	ch <- metaMetric
	ch <- channelLockMetric
	ch <- channelPowerMetric
	ch <- channelSNRMetric
	ch <- channelCorrectedMetric
	ch <- channelUncorrectableMetric
	ch <- channelMetaMetric
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	modem, err := e.Scrape()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(
			upMetric, prometheus.GaugeValue, 0,
		)
		log.Error(err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		upMetric, prometheus.GaugeValue, 1,
	)

	// Connected Metric
	ch <- prometheus.MustNewConstMetric(
		connectedMetric, prometheus.GaugeValue, modem.ConnectivityState,
	)

	// Uptime Metric
	ch <- prometheus.MustNewConstMetric(
		uptimeMetric, prometheus.GaugeValue, modem.Uptime,
	)

	// Modem Meta Metric
	ch <- prometheus.MustNewConstMetric(
		metaMetric, prometheus.GaugeValue, 1,
		e.Host, modem.HardwareVersion, modem.SoftwareVersion,
		modem.MACAddress, modem.SerialNumber,
	)

	// Downstream Channels
	for _, channel := range modem.DownstreamBondedChannels {
		// Lock Metric
		ch <- prometheus.MustNewConstMetric(
			channelLockMetric, prometheus.GaugeValue, channel.LockStatus,
			channel.ChannelID, DOWNSTREAM,
		)

		// Power Metric
		ch <- prometheus.MustNewConstMetric(
			channelPowerMetric, prometheus.GaugeValue, channel.Power,
			channel.ChannelID, DOWNSTREAM,
		)

		// SNR Metric
		ch <- prometheus.MustNewConstMetric(
			channelSNRMetric, prometheus.GaugeValue, channel.SNR,
			channel.ChannelID, DOWNSTREAM,
		)

		// Corrected Errors Metric
		ch <- prometheus.MustNewConstMetric(
			channelCorrectedMetric, prometheus.CounterValue, channel.CorrectedErrors,
			channel.ChannelID, DOWNSTREAM,
		)

		// Uncorrectable Errors Metric
		ch <- prometheus.MustNewConstMetric(
			channelUncorrectableMetric, prometheus.CounterValue, channel.UncorrectableErrors,
			channel.ChannelID, DOWNSTREAM,
		)

		// Meta Metric
		ch <- prometheus.MustNewConstMetric(
			channelMetaMetric, prometheus.GaugeValue, 1,
			channel.ChannelID, channel.Modulation, channel.Frequency,
			"", DOWNSTREAM,
		)
	}

	// Upstream Channels
	for _, channel := range modem.UpstreamBondedChannels {
		// Lock Metric
		ch <- prometheus.MustNewConstMetric(
			channelLockMetric, prometheus.GaugeValue, channel.LockStatus,
			channel.ChannelID, UPSTREAM,
		)

		// Power Metric
		ch <- prometheus.MustNewConstMetric(
			channelPowerMetric, prometheus.GaugeValue, channel.Power,
			channel.ChannelID, UPSTREAM,
		)

		// Meta Metric
		ch <- prometheus.MustNewConstMetric(
			channelMetaMetric, prometheus.GaugeValue, 1,
			channel.ChannelID, channel.USChannelType, channel.Frequency,
			channel.Width, UPSTREAM,
		)
	}
}
