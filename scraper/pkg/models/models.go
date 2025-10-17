package models

type User struct {
	Id   int
	Url  string `json:"url"`
	Name string `json:"name"`
}

type Movie struct {
	Id          int
	Url         string
	Name        string
	Duration    *int
	PosterUrl   *string
	BackdropUrl *string
	Desc        *string
	TrailerUrl  *string
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

type UserAndMovie struct {
	UserId  int
	MovieId int
	Date    string
	IsWatch bool
	Rating  *float32
	IsLoved bool
	Review  *string
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
