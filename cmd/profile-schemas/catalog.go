package main

import "strings"

// entry is one payload editor: a Jamf Pro payload (or one tab of it) and the Apple payload
// definition it is generated from.
type entry struct {
	ID          string // editor id and file name; the Apple YAML file unless File is set
	File        string // Apple YAML file (without .yaml) when it differs from ID
	Title       string // as Jamf Pro names it
	Jamf        string // the Jamf Pro payload (and tab) when Title alone doesn't say it
	Category    string
	Order       int
	Description string // overrides Apple's
	Platforms   string // "ios", "macos" or "ios macos"
	Multiple    *bool  // overrides Apple's "multiple"
	Skip        []string
	Curated     bool // use the curated schema's settings as they are (shape differs from Apple's)
	Interface   bool // Ethernet: one editor for Apple's per-interface payload types
}

func (e entry) file() string {
	if e.File != "" {
		return e.File
	}
	return e.ID
}

func (e entry) on(plat string) bool { return strings.Contains(e.Platforms, plat) }

var yes, no = true, false

// catalog lists the payloads Jamf Pro offers for Computer (macOS) and Mobile Device
// (iOS/iPadOS) Configuration Profiles, as VibeMDM payload editors. Jamf payloads that
// produce several Apple payload types (Security & Privacy, Restrictions, Login Window,
// Network) are one editor per Apple type, named after Jamf's tab.
var catalog = []entry{
	// ---- General ----
	{ID: "com.apple.configurationprofile.identification", Title: "Identification", Category: "General", Order: 10, Platforms: "macos",
		Description: "Pre-fills the user's name and email address in apps and services that use them."},
	{ID: "com.apple.shareddeviceconfiguration", Title: "Lock Screen Message", Category: "General", Order: 10, Platforms: "ios",
		Description: "Shows a message, such as who to return the device to, and an asset tag on the Lock Screen."},
	{ID: "com.apple.airprint", Title: "AirPrint", Category: "General", Order: 20, Platforms: "ios macos",
		Description: "Adds AirPrint printers so users can print without finding them on the network."},
	{ID: "com.apple.airplay", Title: "AirPlay", Category: "General", Order: 30, Platforms: "ios macos",
		Description: "Sets passwords for AirPlay destinations and limits which ones the device can use."},
	{ID: "com.apple.font", Title: "Font", Category: "General", Order: 40, Platforms: "ios macos", Multiple: &yes,
		Description: "Installs a font (TrueType or OpenType)."},
	{ID: "com.apple.mcxprinting", Title: "Printing", Category: "General", Order: 50, Platforms: "macos",
		Description: "Adds printers, sets the default printer and limits printing to the printers listed."},
	{ID: "com.apple.notificationsettings", Title: "Notifications", Category: "General", Order: 60, Platforms: "ios macos",
		Description: "Sets how each app's notifications appear and stops users changing them."},
	{ID: "com.apple.associated-domains", Title: "Associated Domains", Category: "General", Order: 70, Platforms: "macos",
		Description: "Associates apps with websites for universal links, password AutoFill and single sign-on."},
	{ID: "com.apple.domains", Title: "Domains", Category: "General", Order: 80, Platforms: "ios macos",
		Description: "Marks email and web domains as managed, so documents from them open only in managed apps and Safari warns about unmanaged email addresses."},
	{ID: "com.apple.universalaccess", Title: "Accessibility", Category: "General", Order: 90, Platforms: "macos",
		Description: "Sets accessibility options such as zoom, contrast, VoiceOver and mouse keys."},
	{ID: "com.apple.desktop", Title: "Desktop", Jamf: "Restrictions → Desktop", Category: "General", Order: 100, Platforms: "macos",
		Description: "Sets and locks the desktop picture."},
	{ID: "com.apple.dock", Title: "Dock", Category: "General", Order: 110, Platforms: "macos",
		Description: "Sets the Dock's size, position and behaviour, and the apps, folders and links in it."},
	{ID: "com.apple.finder", Title: "Finder", Category: "General", Order: 120, Platforms: "macos",
		Description: "Sets Finder options such as which items appear on the desktop and the warning for emptying the Trash."},
	{ID: "com.apple.screensaver", Title: "Screen Saver", Jamf: "Login Window → Screen Saver / Security & Privacy → General", Category: "General", Order: 130, Platforms: "macos",
		Description: "Requires a password after sleep or screen saver and sets the login window's screen saver."},

	// ---- Security ----
	{ID: "com.apple.mobiledevice.passwordpolicy", Title: "Passcode", Category: "Security", Order: 10, Platforms: "ios macos",
		Description: "Requires a passcode (or a local account password on a Mac) and sets how strong it must be."},
	{ID: "com.apple.preference.security", Title: "Security & Privacy: General", Jamf: "Security & Privacy → General", Category: "Security", Order: 20, Platforms: "macos",
		Description: "Stops users changing the password and controls the lock message."},
	{ID: "com.apple.MCX.FileVault2", Title: "Security & Privacy: FileVault", Jamf: "Security & Privacy → FileVault", Category: "Security", Order: 30, Platforms: "macos",
		Description: "Turns on FileVault and sets how the recovery key is created. Use Settings → FileVault in VibeMDM to escrow keys to the server."},
	{ID: "com.apple.MCX(FileVault2)", Title: "Security & Privacy: FileVault options", Jamf: "Security & Privacy → FileVault", Category: "Security", Order: 31, Platforms: "macos",
		Description: "Stops users turning FileVault off and keeps the prompt from being deferred."},
	{ID: "com.apple.security.FDERecoveryKeyEscrow", Title: "Security & Privacy: FileVault recovery key escrow", Jamf: "Security & Privacy → FileVault", Category: "Security", Order: 32, Platforms: "macos",
		Description: "Escrows the personal recovery key to the MDM server, encrypted with a certificate in this profile."},
	{ID: "com.apple.security.firewall", Title: "Security & Privacy: Firewall", Jamf: "Security & Privacy → Firewall", Category: "Security", Order: 40, Platforms: "macos",
		Description: "Turns on the application firewall, stealth mode and blocks incoming connections per app."},
	{ID: "com.apple.systempolicy.control", Title: "Security & Privacy: Gatekeeper", Jamf: "Security & Privacy → General", Category: "Security", Order: 50, Platforms: "macos",
		Description: "Chooses which apps Gatekeeper allows: App Store only, or App Store and identified developers."},
	{ID: "com.apple.systempolicy.managed", Title: "Security & Privacy: Gatekeeper override", Jamf: "Security & Privacy → General", Category: "Security", Order: 51, Platforms: "macos",
		Description: "Stops users overriding Gatekeeper from the Finder's context menu."},
	{ID: "com.apple.TCC.configuration-profile-policy", Title: "Privacy Preferences Policy Control", Category: "Security", Order: 60, Platforms: "macos",
		Description: "Grants or denies apps access to protected data and features (Full Disk Access, Accessibility, camera, screen recording, …) without asking the user. Needs user-approved MDM."},
	{ID: "com.apple.syspolicy.kernel-extension-policy", Title: "Approved Kernel Extensions", Category: "Security", Order: 70, Platforms: "macos",
		Description: "Lets kernel extensions load without the user approving them, by team ID or bundle ID."},
	{ID: "com.apple.system-extension-policy", Title: "System Extensions", Category: "Security", Order: 80, Platforms: "macos",
		Description: "Allows system extensions (network, endpoint security, drivers) by team ID or bundle ID, and whether users may approve others."},
	{ID: "com.apple.security.smartcard", Title: "Smart Card", Category: "Security", Order: 90, Platforms: "macos",
		Description: "Controls smart card login and pairing, and which certificates are trusted."},
	{ID: "com.apple.webcontent-filter", Title: "Content Filter", Category: "Security", Order: 100, Platforms: "ios macos",
		Description: "Filters web content with Apple's built-in filter (iOS) or a filter app's network extension."},
	{ID: "com.apple.app.lock", Title: "Single App Mode", Category: "Security", Order: 110, Platforms: "ios",
		Description: "Locks the device to one app, for kiosks and shared devices."},
	{ID: "com.apple.security.certificatetransparency", Title: "Certificate Transparency", Category: "Security", Order: 120, Platforms: "ios macos",
		Description: "Turns off certificate transparency enforcement for domains or certificates of your own."},
	{ID: "com.apple.security.certificaterevocation", Title: "Certificate Revocation", Category: "Security", Order: 130, Platforms: "ios",
		Description: "Turns on revocation checking for certificates that would otherwise not be checked."},

	// ---- Restrictions ----
	{ID: "com.apple.applicationaccess", Title: "Restrictions", Category: "Restrictions", Order: 10, Platforms: "ios macos",
		Description: "Turns device features, apps and services on or off. Unset items keep Apple's default (allowed)."},
	{ID: "com.apple.applicationaccess.new", Title: "Restrictions: Applications", Jamf: "Restrictions → Applications", Category: "Restrictions", Order: 20, Platforms: "macos",
		Description: "Allows or blocks apps by path or bundle ID (legacy Parental Controls application restrictions)."},
	{ID: "com.apple.systempreferences", Title: "Restrictions: System Settings", Jamf: "Restrictions → Preferences", Category: "Restrictions", Order: 30, Platforms: "macos",
		Description: "Hides or disables panes in System Settings."},
	{ID: "com.apple.systemuiserver", Title: "Restrictions: Media", Jamf: "Restrictions → Media", Category: "Restrictions", Order: 40, Platforms: "macos",
		Description: "Allows, makes read-only or blocks disks, disc images, CDs and DVDs."},
	{ID: "com.apple.DiscRecording", Title: "Restrictions: Disc burning", Jamf: "Restrictions → Media", Category: "Restrictions", Order: 41, Platforms: "macos",
		Description: "Allows or blocks burning CDs and DVDs in the Finder."},
	{ID: "com.apple.mcxMenuExtras", Title: "Restrictions: Menu bar items", Jamf: "Restrictions → Functionality", Category: "Restrictions", Order: 50, Platforms: "macos",
		Description: "Allows or hides menu bar extras."},
	{ID: "com.apple.MCX(Accounts)", Title: "Restrictions: Accounts", Jamf: "Login Window / Mobility", Category: "Restrictions", Order: 60, Platforms: "macos",
		Description: "Controls guest access and the accounts that can log in."},
	{ID: "com.apple.preference.users", File: "com.apple.preferences.users", Title: "Restrictions: Users & Groups", Jamf: "Restrictions → Preferences", Category: "Restrictions", Order: 70, Platforms: "macos",
		Description: "Stops users changing account details in Users & Groups."},
	{ID: "com.apple.gamed", Title: "Restrictions: Game Center", Jamf: "Restrictions → Applications", Category: "Restrictions", Order: 80, Platforms: "macos",
		Description: "Controls Game Center features (legacy Parental Controls)."},
	{ID: "com.apple.appstore", Title: "Restrictions: App Store", Jamf: "Restrictions → Applications", Category: "Restrictions", Order: 90, Platforms: "macos",
		Description: "Limits the App Store to software updates and stops non-admins installing apps from it."},
	{ID: "com.apple.familycontrols.contentfilter", Title: "Parental Controls: Content filter", Jamf: "Parental Controls", Category: "Restrictions", Order: 100, Platforms: "macos",
		Description: "Filters web content in Safari for the user (legacy Parental Controls)."},
	{ID: "com.apple.familycontrols.timelimits.v2", Title: "Parental Controls: Time limits", Jamf: "Parental Controls", Category: "Restrictions", Order: 110, Platforms: "macos",
		Description: "Limits how long and when the user can use the Mac (legacy Parental Controls)."},
	{ID: "com.apple.NSExtension", Title: "Restrictions: App extensions", Jamf: "Restrictions → Functionality", Category: "Restrictions", Order: 140, Platforms: "macos",
		Description: "Blocks specific app extensions."},
	{ID: "com.apple.networkusagerules", Title: "Network Usage Rules", Category: "Restrictions", Order: 160, Platforms: "ios",
		Description: "Stops apps using cellular data or data roaming."},

	// ---- Network ----
	{ID: "com.apple.wifi.managed", Title: "Wi-Fi", Jamf: "Network (Wi-Fi)", Category: "Network", Order: 10, Platforms: "ios macos",
		Description: "Adds a Wi-Fi network, including WPA2/WPA3 Enterprise (802.1X) with a certificate from this profile."},
	{ID: "com.apple.globalethernet.managed", File: "com.apple.wifi.managed", Title: "Ethernet (802.1X)", Jamf: "Network (Ethernet)", Category: "Network", Order: 20,
		Platforms: "ios macos", Interface: true, Skip: []string{"AutoJoin", "SSID_STR", "HIDDEN_NETWORK", "EncryptionType", "Password", "DisplayedOperatorName",
			"DomainName", "RoamingConsortiumOIs", "ServiceProviderRoamingEnabled", "IsHotspot", "HESSID", "NAIRealmNames", "MCCAndMNCs", "CaptiveBypass",
			"QoSMarkingPolicy", "DisableAssociationMACRandomization", "AllowJoinBeforeFirstUnlock"},
		Description: "802.1X authentication for wired Ethernet."},
	{ID: "com.apple.vpn.managed", Title: "VPN", Category: "Network", Order: 30, Platforms: "ios macos",
		Description: "Adds a VPN connection: the built-in IKEv2, L2TP or IPSec client, or a VPN app."},
	{ID: "com.apple.vpn.managed.applayer", Title: "Per-App VPN", Category: "Network", Order: 40, Platforms: "ios macos", Multiple: &yes,
		Description: "A VPN connection used only by the apps that are assigned to it."},
	{ID: "com.apple.vpn.managed.appmapping", Title: "Per-App VPN app mapping", Category: "Network", Order: 41, Platforms: "macos",
		Description: "Assigns Mac apps to a per-app VPN by bundle ID and code requirement."},
	{ID: "com.apple.SystemConfiguration", Title: "Proxies", Category: "Network", Order: 50, Platforms: "macos",
		Description: "Sets HTTP, HTTPS, SOCKS and automatic (PAC) proxies for the network services on the Mac."},
	{ID: "com.apple.proxy.http.global", Title: "Global HTTP Proxy", Category: "Network", Order: 60, Platforms: "ios",
		Description: "Sends all HTTP traffic through a proxy. Supervised only."},
	{ID: "com.apple.dnsSettings.managed", Title: "DNS Settings", Category: "Network", Order: 70, Platforms: "ios macos", Multiple: &yes,
		Description: "Uses encrypted DNS (DNS over HTTPS or TLS) for the whole device or for chosen domains."},
	{ID: "com.apple.dnsProxy.managed", Title: "DNS Proxy", Category: "Network", Order: 80, Platforms: "ios macos",
		Description: "Sends DNS traffic to a DNS proxy app's network extension."},
	{ID: "com.apple.relay.managed", Title: "Relay", Category: "Network", Order: 90, Platforms: "ios macos", Multiple: &yes,
		Description: "Sends app or domain traffic through HTTP/3 or HTTP/2 relay servers instead of a VPN."},
	{ID: "com.apple.cellular", Title: "Cellular", Category: "Network", Order: 100, Platforms: "ios",
		Description: "Sets the cellular access point (APN) and its authentication."},
	{ID: "com.apple.apn.managed", Title: "APN", Category: "Network", Order: 105, Platforms: "ios",
		Description: "Sets the cellular access point name (legacy; use Cellular instead)."},
	{ID: "com.apple.cellularprivatenetwork.managed", Title: "Cellular Private Network", Category: "Network", Order: 110, Platforms: "ios", Multiple: &yes,
		Description: "Configures a private 5G or LTE network."},
	{ID: "com.apple.AssetCache.managed", Title: "Content Caching", Category: "Network", Order: 120, Platforms: "macos",
		Description: "Configures the content caching service on a Mac that serves Apple software and iCloud data to devices on the network."},

	// ---- Certificates ----
	{ID: "com.apple.security.pkcs1", Title: "Certificate", Category: "Certificates", Order: 10, Platforms: "ios macos", Multiple: &yes,
		Description: "Installs a CA or server certificate (DER or PEM) so the device trusts servers that use it."},
	{ID: "com.apple.security.root", Title: "Certificate (root)", Category: "Certificates", Order: 11, Platforms: "ios macos", Multiple: &yes,
		Description: "Installs a root certificate authority."},
	{ID: "com.apple.security.pem", Title: "Certificate (PEM)", Category: "Certificates", Order: 12, Platforms: "ios macos", Multiple: &yes,
		Description: "Installs a certificate in PEM format."},
	{ID: "com.apple.security.pkcs12", Title: "Certificate (identity)", Jamf: "Certificate (.p12)", Category: "Certificates", Order: 20, Platforms: "ios macos", Multiple: &yes,
		Description: "Installs a certificate with its private key (.p12 or .pfx), for example for Wi-Fi, VPN or email authentication."},
	{ID: "com.apple.security.scep", Title: "SCEP", Category: "Certificates", Order: 30, Platforms: "ios macos", Multiple: &yes,
		Description: "Has the device request a certificate from a SCEP server, so the private key never leaves the device."},
	{ID: "com.apple.security.acme", Title: "ACME Certificate", Category: "Certificates", Order: 40, Platforms: "ios macos", Multiple: &yes,
		Description: "Has the device request a certificate from an ACME server, with hardware-bound keys and device attestation."},
	{ID: "com.apple.ADCertificate.managed", Title: "AD Certificate", Category: "Certificates", Order: 50, Platforms: "macos", Multiple: &yes,
		Description: "Requests a certificate from Active Directory Certificate Services."},
	{ID: "com.apple.security.certificatepreference", Title: "Certificate Preference", Category: "Certificates", Order: 60, Platforms: "macos", Multiple: &yes,
		Description: "Chooses which certificate the Mac prefers for a name or email address."},
	{ID: "com.apple.security.identitypreference", Title: "Identity Preference", Category: "Certificates", Order: 70, Platforms: "macos", Multiple: &yes,
		Description: "Chooses which identity certificate the Mac uses for a server."},

	// ---- Accounts ----
	{ID: "com.apple.mail.managed", Title: "Mail", Category: "Accounts", Order: 10, Platforms: "ios macos",
		Description: "Adds an IMAP or POP email account."},
	{ID: "com.apple.eas.account", Title: "Exchange ActiveSync", Category: "Accounts", Order: 20, Platforms: "ios",
		Description: "Adds a Microsoft Exchange account for mail, contacts, calendars, reminders and notes."},
	{ID: "com.apple.ews.account", Title: "Exchange Web Services", Category: "Accounts", Order: 21, Platforms: "macos",
		Description: "Adds a Microsoft Exchange account to Mail, Contacts and Calendar."},
	{ID: "com.apple.google-oauth", Title: "Google Account", Category: "Accounts", Order: 30, Platforms: "ios",
		Description: "Adds a Google account; the user signs in to finish setting it up."},
	{ID: "com.apple.ldap.account", Title: "LDAP", Category: "Accounts", Order: 40, Platforms: "ios macos",
		Description: "Adds an LDAP directory for looking up contacts."},
	{ID: "com.apple.caldav.account", Title: "Calendar", Category: "Accounts", Order: 50, Platforms: "ios macos",
		Description: "Adds a CalDAV calendar account."},
	{ID: "com.apple.carddav.account", Title: "Contacts", Category: "Accounts", Order: 60, Platforms: "ios macos",
		Description: "Adds a CardDAV contacts account."},
	{ID: "com.apple.subscribedcalendar.account", Title: "Subscribed Calendars", Category: "Accounts", Order: 70, Platforms: "ios",
		Description: "Subscribes to a read-only calendar."},

	// ---- Single Sign-On ----
	{ID: "com.apple.extensiblesso", Title: "Single Sign-On Extensions", Category: "Single Sign-On", Order: 10, Platforms: "ios macos",
		Description: "Configures an SSO extension app (redirect or credential), such as Microsoft Enterprise SSO or Okta, including Platform SSO on macOS."},
	{ID: "com.apple.extensiblesso(kerberos)", Title: "Single Sign-On Extensions: Kerberos", Jamf: "Single Sign-On Extensions (Kerberos)", Category: "Single Sign-On", Order: 20, Platforms: "ios macos",
		Description: "Configures Apple's built-in Kerberos SSO extension for Active Directory sign-in."},
	{ID: "com.apple.sso", Title: "Single Sign-On", Category: "Single Sign-On", Order: 30, Platforms: "ios",
		Description: "Kerberos single sign-on for apps and URLs (legacy; use Single Sign-On Extensions on newer devices)."},

	// ---- Apps and Home Screen ----
	{ID: "com.apple.homescreenlayout", Title: "Home Screen Layout", Category: "Apps and Home Screen", Order: 10, Platforms: "ios", Curated: true,
		Description: "Sets and locks the order of apps, folders and web clips on the Home Screen and Dock. Supervised only."},
	{ID: "com.apple.webClip.managed", Title: "Web Clips", Category: "Apps and Home Screen", Order: 20, Platforms: "ios macos",
		Description: "Puts a website shortcut on the Home Screen (iOS) or in the Dock (macOS)."},

	// ---- System (macOS) ----
	{ID: "com.apple.MCX(EnergySaver)", Title: "Energy Saver", Category: "System", Order: 20, Platforms: "macos",
		Description: "Sets sleep, wake and power settings, and a schedule to start up, shut down or sleep."},
	{ID: "com.apple.MCX.TimeMachine", Title: "Time Machine", Category: "System", Order: 30, Platforms: "macos",
		Description: "Sets the Time Machine backup destination and what is backed up."},
	{ID: "com.apple.MCX(TimeServer)", Title: "Time Server", Jamf: "Date & Time", Category: "System", Order: 40, Platforms: "macos",
		Description: "Sets the network time server and time zone."},
	{ID: "com.apple.servicemanagement", Title: "Managed Login Items", Category: "System", Order: 50, Platforms: "macos",
		Description: "Keeps launch agents, daemons and login items from managed apps turned on in System Settings, matched by team ID, bundle ID, label or path."},
	{ID: "com.apple.systemmigration", Title: "System Migration", Category: "System", Order: 70, Platforms: "macos",
		Description: "Tells Migration Assistant which files to move when migrating from Windows."},
	{ID: "com.apple.fileproviderd", Title: "File Provider", Category: "System", Order: 80, Platforms: "macos",
		Description: "Controls which File Provider extensions may show files outside their sync folder."},
	{ID: "com.apple.xsan", Title: "Xsan", Category: "System", Order: 90, Platforms: "macos",
		Description: "Joins the Mac to an Xsan volume."},
	{ID: "com.apple.xsan.preferences", Title: "Xsan preferences", Category: "System", Order: 91, Platforms: "macos",
		Description: "Xsan client preferences."},
	{ID: "com.apple.DirectoryService.managed", Title: "Directory", Category: "System", Order: 100, Platforms: "macos",
		Description: "Binds the Mac to Active Directory and sets mobile account, admin group and home folder options."},

	// ---- Users and Login (macOS) ----
	{ID: "com.apple.loginwindow", Title: "Login Window", Category: "Users and Login", Order: 10, Platforms: "macos",
		Description: "Sets the login window's banner, which users and buttons it shows, and auto-login."},
	{ID: "loginwindow", Title: "Login Window: Options", Jamf: "Login Window → Options", Category: "Users and Login", Order: 11, Platforms: "macos",
		Description: "More login window options for the user."},
	{ID: "com.apple.loginitems.managed", Title: "Login Items", Category: "Users and Login", Order: 20, Platforms: "macos",
		Description: "Opens apps, documents and network shares when the user logs in."},
	{ID: "com.apple.MCX(Mobility)", Title: "Mobility", Category: "Users and Login", Order: 30, Platforms: "macos",
		Description: "Creates mobile accounts for directory users and syncs their home folders."},
	{ID: "com.apple.screensaver.user", Title: "Screen Saver (user)", Category: "Users and Login", Order: 40, Platforms: "macos",
		Description: "Sets the user's screen saver and when it starts."},
	{ID: "com.apple.SetupAssistant.managed", Title: "Skip Setup Items", Jamf: "Skip Setup Items", Category: "Users and Login", Order: 50, Platforms: "ios macos",
		Description: "Skips Setup Assistant screens shown after updates or when a user logs in for the first time."},

	// ---- Education ----
	{ID: "com.apple.education", Title: "Education", Category: "Education", Order: 10, Platforms: "ios macos",
		Description: "Configures Classroom: the organization, groups, teachers and students."},
}

// titles name settings whose Apple key reads badly as a title ("tilesize"), using the
// wording of the macOS and iOS settings (and Jamf Pro's, for privacy services). Keys are
// "<editor id>/<path>", "[]" marking list items.
var titles = map[string]string{
	"com.apple.dock/tilesize":                   "Icon size",
	"com.apple.dock/size-immutable":             "Lock icon size",
	"com.apple.dock/magnification":              "Magnification",
	"com.apple.dock/magnify-immutable":          "Lock magnification",
	"com.apple.dock/largesize":                  "Magnified icon size",
	"com.apple.dock/magsize-immutable":          "Lock magnified icon size",
	"com.apple.dock/orientation":                "Position on screen",
	"com.apple.dock/position-immutable":         "Lock position",
	"com.apple.dock/mineffect":                  "Minimize windows using",
	"com.apple.dock/mineffect-immutable":        "Lock minimize effect",
	"com.apple.dock/windowtabbing":              "Prefer tabs when opening documents",
	"com.apple.dock/windowtabbing-immutable":    "Lock tab preference",
	"com.apple.dock/dblclickbehavior":           "Double-click a window's title bar to",
	"com.apple.dock/dblclickbehavior-immutable": "Lock title bar double-click",
	"com.apple.dock/minimize-to-application":    "Minimize windows into application icon",
	"com.apple.dock/minintoapp-immutable":       "Lock minimize into application icon",
	"com.apple.dock/launchanim":                 "Animate opening applications",
	"com.apple.dock/launchanim-immutable":       "Lock opening animation",
	"com.apple.dock/autohide":                   "Automatically hide and show the Dock",
	"com.apple.dock/autohide-immutable":         "Lock auto-hide",
	"com.apple.dock/show-process-indicators":    "Show indicators for open applications",
	"com.apple.dock/showindicators-immutable":   "Lock open-application indicators",
	"com.apple.dock/show-recents":               "Show suggested and recent apps in Dock",
	"com.apple.dock/showrecents-immutable":      "Lock recent apps setting",
	"com.apple.dock/contents-immutable":         "Lock Dock contents",
	"com.apple.dock/MCXDockSpecialFolders":      "Special folders (My Applications, Documents, Network Home)",
	"com.apple.dock/AllowDockFixupOverride":     "Allow Dock fixup override",
	"com.apple.dock/static-only":                "Use only the items below (no user items)",
	"com.apple.dock/static-apps":                "Apps (locked)",
	"com.apple.dock/static-others":              "Files and folders (locked)",
	"com.apple.dock/persistent-apps":            "Apps",
	"com.apple.dock/persistent-others":          "Files and folders",

	"com.apple.TCC.configuration-profile-policy/Services":                              "Services",
	"com.apple.TCC.configuration-profile-policy/Services.Accessibility":                "Accessibility",
	"com.apple.TCC.configuration-profile-policy/Services.AddressBook":                  "Contacts",
	"com.apple.TCC.configuration-profile-policy/Services.AppleEvents":                  "Apple Events (automation)",
	"com.apple.TCC.configuration-profile-policy/Services.BluetoothAlways":              "Bluetooth",
	"com.apple.TCC.configuration-profile-policy/Services.Calendar":                     "Calendars",
	"com.apple.TCC.configuration-profile-policy/Services.Camera":                       "Camera",
	"com.apple.TCC.configuration-profile-policy/Services.FileProviderPresence":         "File provider presence",
	"com.apple.TCC.configuration-profile-policy/Services.ListenEvent":                  "Input Monitoring",
	"com.apple.TCC.configuration-profile-policy/Services.MediaLibrary":                 "Media & Apple Music",
	"com.apple.TCC.configuration-profile-policy/Services.Microphone":                   "Microphone",
	"com.apple.TCC.configuration-profile-policy/Services.Photos":                       "Photos",
	"com.apple.TCC.configuration-profile-policy/Services.PostEvent":                    "Post event (control the computer)",
	"com.apple.TCC.configuration-profile-policy/Services.Reminders":                    "Reminders",
	"com.apple.TCC.configuration-profile-policy/Services.ScreenCapture":                "Screen & System Audio Recording",
	"com.apple.TCC.configuration-profile-policy/Services.SpeechRecognition":            "Speech Recognition",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyAllFiles":         "Full Disk Access",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyAppBundles":       "App Management",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyAppData":          "App data (other apps' data)",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyDesktopFolder":    "Desktop folder",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyDocumentsFolder":  "Documents folder",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyDownloadsFolder":  "Downloads folder",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyNetworkVolumes":   "Network volumes",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicyRemovableVolumes": "Removable volumes",
	"com.apple.TCC.configuration-profile-policy/Services.SystemPolicySysAdminFiles":    "Administrator files",

	"com.apple.systemuiserver/mount-controls":   "Media mounting",
	"com.apple.systemuiserver/unmount-controls": "Media unmounting",
	"com.apple.systemuiserver/logout-eject":     "Eject at logout",
	"com.apple.universalaccess/contrast":        "Display contrast",
	"com.apple.universalaccess/grayscale":       "Use grayscale",
	"com.apple.loginwindow/SHOWFULLNAME":        "Show name and password fields (instead of a list of users)",
	"com.apple.xsan/fsnameservers":              "File system name servers",
}
