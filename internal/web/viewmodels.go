package web

type Page struct {
	Title, CSRF, Flash, Error string
	Data                      any
}
