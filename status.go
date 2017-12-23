package main

import (
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type efesStatus struct {
	devices []deviceStatus
}

type deviceStatus struct {
	Device
	Hostname   string
	HostStatus string
	UpdatedAt  time.Time
}

func (d deviceStatus) Size() string {
	if d.BytesTotal == nil {
		return ""
	}
	return humanize.Comma(*d.BytesTotal / G)
}

func (d deviceStatus) Used() string {
	if d.BytesUsed == nil {
		return ""
	}
	return humanize.Comma(*d.BytesUsed / G)
}

func (d deviceStatus) Free() string {
	if d.BytesFree == nil {
		return ""
	}
	return humanize.Comma(*d.BytesFree / G)
}

func (d deviceStatus) Use() string {
	if d.BytesUsed == nil || d.BytesTotal == nil {
		return ""
	}
	use := (*d.BytesUsed * 100) / *d.BytesTotal
	return strconv.FormatInt(use, 10)
}

func (d deviceStatus) IO() string {
	if d.IoUtilization == nil {
		return ""
	}
	return strconv.FormatInt(*d.IoUtilization, 10)
}

func (s *efesStatus) Print() {
	// Sum totals
	var totalUsed, totalSize int64 // in MB
	for _, d := range s.devices {
		if d.BytesUsed != nil {
			totalUsed += *d.BytesUsed
		}
		if d.BytesTotal != nil {
			totalSize += *d.BytesTotal
		}
	}
	totalFree := totalSize - totalUsed
	var totalUse int64
	if totalSize == 0 {
		totalUse = 0
	} else {
		totalUse = (100 * totalUsed) / totalSize
	}

	// Convert to GB
	totalUsed /= G
	totalFree /= G
	totalSize /= G

	// Setup table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(false)
	table.SetAlignment(tablewriter.ALIGN_RIGHT)
	table.SetFooterAlignment(tablewriter.ALIGN_RIGHT)
	table.SetHeader([]string{
		"Host",
		"Status",
		"Device",
		"Status",
		"Size (G)",
		"Used (G)",
		"Free (G)",
		"Use %",
		"IO %",
		"Last update",
	})
	table.SetFooter([]string{
		"", "", "",
		"Total:",
		humanize.Comma(totalSize),
		humanize.Comma(totalUsed),
		humanize.Comma(totalFree),
		strconv.FormatInt(totalUse, 10),
		"", "",
	})

	// Add data to the table
	now := time.Now().UTC()
	data := make([][]string, len(s.devices))
	for i, d := range s.devices {
		data[i] = []string{
			d.Hostname,
			d.HostStatus,
			strconv.FormatInt(d.Devid, 10),
			d.Status,
			d.Size(),
			d.Used(),
			d.Free(),
			d.Use(),
			d.IO(),
			now.Sub(d.UpdatedAt).Truncate(time.Second).String(),
		}

	}
	table.AppendBulk(data) // Add Bulk Data
	table.Render()
}

func (c *Client) Status(sortBy string) (*efesStatus, error) {
	ret := &efesStatus{
		devices: make([]deviceStatus, 0),
	}
	var devices GetDevices
	err := c.request(http.MethodGet, "get-devices", nil, &devices)
	if err != nil {
		return nil, err
	}
	var hosts GetHosts
	err = c.request(http.MethodGet, "get-hosts", nil, &hosts)
	if err != nil {
		return nil, err
	}
	hostsByID := make(map[int64]Host)
	for _, h := range hosts.Hosts {
		hostsByID[h.Hostid] = h
	}
	for _, d := range devices.Devices {
		if d.Status == "dead" {
			continue
		}
		var hostname string
		var hostStatus string
		if h, ok := hostsByID[d.Hostid]; ok {
			hostname = h.Hostname
			hostStatus = h.Status
		}
		ds := deviceStatus{
			Device:     d,
			Hostname:   hostname,
			HostStatus: hostStatus,
			UpdatedAt:  time.Unix(d.UpdatedAt, 0),
		}
		ret.devices = append(ret.devices, ds)
	}
	switch sortBy {
	case "host":
		sort.Sort(byHostname{ret.devices})
	case "device":
		sort.Sort(byDevID{ret.devices})
	case "size":
		sort.Sort(bySize{ret.devices})
	case "used":
		sort.Sort(byUsed{ret.devices})
	case "free":
		sort.Sort(byFree{ret.devices})
	default:
		c.log.Warningln("Sort key is not valid:", sortBy)
	}
	return ret, nil
}
