package appstore

// App is one installable service. Everything that reaches apt-get comes from
// this fixed table; the API only ever receives an app ID.
type App struct {
	ID          string
	Name        string
	Category    string
	Description string
	Icon        string // lucide-react icon name
	Color       string // accent used by the UI tile
	Check       []string
	Packages    []string
	// OptionalAny are tried in order until one installs (package names differ per distribution).
	OptionalAny []string
	Unit        string // systemd unit; empty for tools without a daemon
	// Versions (newest first) makes the app selectable by version. Versions the
	// distribution does not ship are installed from Repo.
	Versions     []string
	VersionPkgs  func(label string, viaRepo bool) []string
	VersionCheck string // package whose apt candidate shows which version the distro offers (nodejs, mariadb-server); empty = "<VersionPkgs[0]>"
	VersionUnit  func(label string) string
	Repo         *RepoDef
	// Multi lets several versions be installed side by side (PHP).
	Multi bool
	// Required apps back RPanel itself and can never be removed from here.
	Required bool
}

const (
	CatWeb       = "Web Server"
	CatRuntime   = "Runtime"
	CatDatabase  = "Database"
	CatContainer = "Container"
	CatSecurity  = "Security"
	CatTools     = "Tools"
)

// Catalog is the ordered list shown in the store.
var Catalog = []App{
	{
		ID: "nginx", Name: "Nginx", Category: CatWeb, Icon: "Globe", Color: "#009639",
		Description: "High-performance web server and reverse proxy. Serves every website you create in RPanel.",
		Check:       []string{"nginx"}, Packages: []string{"nginx"}, Unit: "nginx", Required: true,
	},
	{
		ID: "php", Name: "PHP-FPM", Category: CatRuntime, Icon: "FileCode2", Color: "#777bb3",
		Description: "PHP with FPM and the common extensions (MySQL, PostgreSQL, cURL, GD, mbstring, XML, Zip). Several versions can run side by side.",
		Multi:       true,
		Versions:    []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"},
		VersionPkgs: phpPackages,
		VersionUnit: func(v string) string { return "php" + v + "-fpm" },
		Repo:        phpRepo,
	},
	{
		ID: "composer", Name: "Composer", Category: CatTools, Icon: "Package", Color: "#885630",
		Description: "Dependency manager for PHP projects.",
		Check:       []string{"composer"}, Packages: []string{"composer"},
	},
	{
		ID: "nodejs", Name: "Node.js", Category: CatRuntime, Icon: "Hexagon", Color: "#5fa04e",
		Description: "JavaScript runtime with npm. Run Node apps behind a reverse-proxy website.",
		Check:       []string{"nodejs"}, Packages: []string{"nodejs", "npm"},
		Versions: []string{"26", "24", "22", "20"}, VersionCheck: "nodejs", Repo: nodeRepo,
		VersionPkgs: func(_ string, viaRepo bool) []string {
			if viaRepo {
				return []string{"nodejs"} // NodeSource bundles npm and conflicts with Ubuntu's npm package
			}
			return []string{"nodejs", "npm"}
		},
	},
	{
		ID: "mariadb", Name: "MariaDB", Category: CatDatabase, Icon: "Database", Color: "#c0765a",
		Description: "MySQL-compatible relational database. After installing, add it in Databases to manage it from the panel.",
		Check:       []string{"mariadb-server"}, Packages: []string{"mariadb-server"}, Unit: "mariadb",
		Versions: []string{"12.3", "11.8", "11.4", "10.11"}, VersionCheck: "mariadb-server", Repo: mariadbRepo,
		VersionPkgs: func(string, bool) []string { return []string{"mariadb-server"} },
	},
	{
		ID: "postgresql", Name: "PostgreSQL", Category: CatDatabase, Icon: "Database", Color: "#336791",
		Description: "Advanced open-source relational database. RPanel stores its own data here.",
		Check:       []string{"postgresql"}, Packages: []string{"postgresql"}, Unit: "postgresql", Required: true,
	},
	{
		ID: "redis", Name: "Redis", Category: CatDatabase, Icon: "DatabaseZap", Color: "#d82c20",
		Description: "In-memory data store for caching and queues. RPanel keeps login sessions here.",
		Check:       []string{"redis-server"}, Packages: []string{"redis-server"}, Unit: "redis-server", Required: true,
	},
	{
		ID: "memcached", Name: "Memcached", Category: CatDatabase, Icon: "Zap", Color: "#3b82a0",
		Description: "Simple, fast in-memory object cache.",
		Check:       []string{"memcached"}, Packages: []string{"memcached"}, Unit: "memcached",
	},
	{
		ID: "docker", Name: "Docker", Category: CatContainer, Icon: "Container", Color: "#2496ed",
		Description: "Container engine with the Compose plugin. Unlocks the Docker page.",
		Check:       []string{"docker.io", "docker-ce"}, Packages: []string{"docker.io"},
		OptionalAny: []string{"docker-compose-v2", "docker-compose-plugin"}, Unit: "docker",
	},
	{
		ID: "certbot", Name: "Certbot", Category: CatSecurity, Icon: "ShieldCheck", Color: "#f59e0b",
		Description: "Free Let's Encrypt SSL certificates for your websites.",
		Check:       []string{"certbot"}, Packages: []string{"certbot"},
	},
	{
		ID: "fail2ban", Name: "Fail2ban", Category: CatSecurity, Icon: "ShieldAlert", Color: "#b91c1c",
		Description: "Bans IP addresses that show malicious behaviour such as repeated SSH login failures.",
		Check:       []string{"fail2ban"}, Packages: []string{"fail2ban"}, Unit: "fail2ban",
	},
	{
		ID: "supervisor", Name: "Supervisor", Category: CatTools, Icon: "Layers", Color: "#6b7280",
		Description: "Process manager that keeps long-running apps and queue workers alive.",
		Check:       []string{"supervisor"}, Packages: []string{"supervisor"}, Unit: "supervisor",
	},
	{
		ID: "git", Name: "Git", Category: CatTools, Icon: "GitBranch", Color: "#f05032",
		Description: "Version control, to deploy code straight onto the server.",
		Check:       []string{"git"}, Packages: []string{"git"},
	},
}

func find(id string) *App {
	for i := range Catalog {
		if Catalog[i].ID == id {
			return &Catalog[i]
		}
	}
	return nil
}
