package software

// Base stack installed on first run and whenever it is still missing.
const (
	BasePHPVersion     = "8.4"
	BaseMariaDBVersion = "11.4"
)

type StackPkg struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

func BaseStack() []StackPkg {
	return []StackPkg{
		{Name: "nginx", Title: "Nginx", Version: ""},
		{Name: "apache", Title: "Apache", Version: ""},
		{Name: "php", Title: "PHP-FPM", Version: BasePHPVersion},
		{Name: "mariadb", Title: "MariaDB", Version: BaseMariaDBVersion},
	}
}
