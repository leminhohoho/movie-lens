package models

type User struct {
	Id   int
	Url  string
	Name string
}

type Movie struct {
	Id          int
	Url         string
	Name        string
	Duration    int
	PosterUrl   *string
	BackdropUrl *string
}

type Cast struct {
	Id   int
	Url  string
	Name string
}
