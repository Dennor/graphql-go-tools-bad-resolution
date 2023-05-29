package graph

import "github.com/Dennor/graphql-go-tools-bad-resolution/author/graph/model"

var authors = []*model.Author{
	{
		Name: "John Doe",
		Media: []model.Media{
			&model.BookMedia{
				ID: "book-id",
				Media: &model.Book{
					ID: "book-id",
				},
			},
			&model.MovieMedia{
				ID: "movie-id",
				Media: &model.Movie{
					ID: "movie-id",
				},
			},
		},
	},
}
