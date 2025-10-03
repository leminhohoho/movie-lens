package models

import "time"

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

type UsersAndMovies struct {
	UserId    int
	MovieId   int
	WatchDate time.Time
	IsLoved   bool
	Review    *string
}
