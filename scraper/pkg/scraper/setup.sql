DROP TABLE IF EXISTS users;
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);


DROP TABLE IF EXISTS movies;
CREATE TABLE movies (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    duration INT,
    poster_url TEXT,
    backdrop_url TEXT
);


DROP TABLE IF EXISTS crews;
CREATE TABLE crews (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL
);


DROP TABLE IF EXISTS crews_and_movies;
CREATE TABLE crews_and_movies (
    crew_id INTEGER NOT NULL,
    movie_id INTEGER NOT NULL
);


DROP TABLE IF EXISTS genres;
CREATE TABLE genres (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);


DROP TABLE IF EXISTS genres_and_movies;
CREATE TABLE genres_and_movies (
    genre_id INTEGER NOT NULL,
    movie_id INTEGER NOT NULL
);


DROP TABLE IF EXISTS themes;
CREATE TABLE themes (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);


DROP TABLE IF EXISTS themes_and_movies;
CREATE TABLE themes_and_movies (
    theme_id INTEGER NOT NULL,
    movie_id INTEGER NOT NULL
);


DROP TABLE IF EXISTS releases;
CREATE TABLE releases (
    movie_id INTEGER NOT NULL,
    date TEXT NOT NULL,
    country TEXT NOT NULL,
    age_rating TEXT,
    release_type TEXT NOT NULL
);


DROP TABLE IF EXISTS studios;
CREATE TABLE studios (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);


DROP TABLE IF EXISTS studios_and_movies;
CREATE TABLE studios_and_movies (
    studio_id INTEGER NOT NULL,
    movie_id INTEGER NOT NULL
);


DROP TABLE IF EXISTS countries_and_movies;
CREATE TABLE countries_and_movies (movie_id INTEGER NOT NULL, country TEXT NOT NULL);


DROP TABLE IF EXISTS languages_and_movies;
CREATE TABLE languages_and_movies (
    movie_id INTEGER NOT NULL,
    language TEXT NOT NULL,
    is_primary INTEGER NOT NULL CHECK (is_primary IN (0, 1))
);


DROP TABLE IF EXISTS users_and_movies;
CREATE TABLE users_and_movies (
    user_id INTEGER NOT NULL,
    movie_id INTEGER NOT NULL,
    date TEXT NOT NULL,
    is_watch INTEGER NOT NULL CHECK (is_loved IN (0, 1)),
    rating REAL,
    is_loved INTEGER NOT NULL CHECK (is_loved IN (0, 1)),
    review TEXT
);
