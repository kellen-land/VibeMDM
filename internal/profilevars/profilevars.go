// Package profilevars fills Jamf Pro-style payload variables ($SERIALNUMBER, $EMAIL, …) in a
// configuration profile with one device's values, just before it is sent to that device.
package profilevars

import (
	"html"
	"sort"
	"strings"

	"github.com/vibemdm/vibemdm/internal/model"
)

// Names lists the supported variables (also shown in the console).
var Names = []string{"$COMPUTERNAME", "$DEVICENAME", "$SERIALNUMBER", "$UDID", "$MACADDRESS", "$ASSETTAG", "$USERNAME", "$FULLNAME",
	"$REALNAME", "$EMAIL", "$PHONE", "$POSITION", "$DEPARTMENTNAME", "$BUILDINGNAME", "$ROOM", "$SITENAME"}

// ForComputer returns a Mac's values. $USERNAME is the assigned user, else the last user.
func ForComputer(c *model.Computer) map[string]string {
	user := firstNonEmpty(c.AssignedUser, c.LastUser)
	full := firstNonEmpty(c.AssignedRealName, c.LastUserFullName)
	return map[string]string{
		"$COMPUTERNAME": c.Name, "$DEVICENAME": c.Name, "$SERIALNUMBER": c.SerialNumber, "$UDID": c.UDID, "$MACADDRESS": c.MACAddress,
		"$USERNAME": user, "$FULLNAME": full, "$REALNAME": full, "$EMAIL": c.AssignedEmail, "$PHONE": c.AssignedPhone,
		"$POSITION": c.AssignedPosition, "$DEPARTMENTNAME": c.Department, "$BUILDINGNAME": c.Building, "$ROOM": c.Room, "$SITENAME": c.Site,
	}
}

// ForMobileDevice returns an iPhone's or iPad's values.
func ForMobileDevice(d *model.MobileDevice) map[string]string {
	return map[string]string{
		"$COMPUTERNAME": d.Name, "$DEVICENAME": d.Name, "$SERIALNUMBER": d.SerialNumber, "$UDID": d.UDID, "$MACADDRESS": d.WiFiMAC,
		"$ASSETTAG": d.AssetTag, "$USERNAME": d.AssignedUser, "$FULLNAME": d.AssignedRealName, "$REALNAME": d.AssignedRealName,
		"$EMAIL": d.AssignedEmail, "$PHONE": d.PhoneNumber,
		"$DEPARTMENTNAME": d.Department, "$BUILDINGNAME": d.Building, "$ROOM": d.Room,
	}
}

// Expand replaces the variables in a profile's XML with XML-escaped values. Unknown values
// become empty, as in Jamf Pro. Longer names are replaced first ($FULLNAME before …).
func Expand(xml string, vars map[string]string) string {
	if !strings.Contains(xml, "$") {
		return xml
	}
	names := append([]string{}, Names...)
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	pairs := make([]string, 0, len(names)*2)
	for _, n := range names {
		pairs = append(pairs, n, html.EscapeString(vars[n]))
	}
	return strings.NewReplacer(pairs...).Replace(xml)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
