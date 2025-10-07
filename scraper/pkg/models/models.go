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
	Duration    *int
	PosterUrl   *string
	BackdropUrl *string
}

type Crew struct {
	Id   int
	Url  string
	Name string
	Role string
}

type CrewsAndMovies struct {
	CrewId  int
	MovieId int
}

type UsersAndMovies struct {
	UserId    int
	MovieId   int
	WatchDate time.Time
	IsLoved   bool
	Review    *string
}

type Genre struct {
	Id   int
	Url  string
	Name string
}

type GenresAndMovies struct {
	GenreId int
	MovieId int
}

type Theme struct {
	Id   int
	Url  string
	Name string
}

type ThemesAndMovies struct {
	ThemeId int
	MovieId int
}

type Studio struct {
	Id   int
	Url  string
	Name string
}

type StudiosAndMovies struct {
	StudioId int
	MovieId  int
}

type CountriesAndMovies struct {
	MovieId int
	Country string
}

type LanguagesAndMovies struct {
	MovieId   int
	Language  string
	IsPrimary bool
}

type Release struct {
	MovieId     int
	Date        string
	Country     string
	AgeRating   *string
	ReleaseType string
}
