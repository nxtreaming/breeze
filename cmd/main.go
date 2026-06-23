package main

import (
	"runtime"

	"github.com/nelthaarion/breeze"
	middleware "github.com/nelthaarion/breeze/middlewares"
	"github.com/nelthaarion/breeze/swagger"
)

// ─── Request / Response types ────────────────────────────────────────────────

type CreateUserRequest struct {
	Name  string `json:"name"  description:"Full name of the user"`
	Email string `json:"email" description:"Email address"`
	Age   int    `json:"age"   description:"Age in years"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserListResponse struct {
	Users []UserResponse `json:"users"`
	Total int            `json:"total"`
}

type UserQueryParams struct {
	Page  int    `json:"page"  description:"Page number (default 1)"  omitempty:"true"`
	Limit int    `json:"limit" description:"Items per page (default 20)" omitempty:"true"`
	Sort  string `json:"sort"  description:"Sort field"                omitempty:"true"`
}

type UserPathParams struct {
	ID string `json:"id" description:"User UUID"`
}

// ─── main ────────────────────────────────────────────────────────────────────

func main() {
	router := breeze.NewRouter()

	// ── Swagger middleware (must be registered before routes) ────────────────
	//
	// SwaggerMiddleware does three things:
	//   1. Calls swagger.Enable() + swagger.SetInfo(…)
	//   2. Registers GET /swagger.json → raw OpenAPI JSON
	//   3. Registers GET /swagger      → Swagger UI HTML
	//
	// It is wired as a global middleware via router.Use so it runs on every
	// request, but its only runtime work is ctx.Next() — all real work
	// (doc registration, spec generation) happens at startup.
	router.Use(middleware.SwaggerMiddleware(router, middleware.SwaggerOptions{
		Title:       "Breeze Example API",
		Version:     "1.0.0",
		Description: "A demonstration of the Breeze swagger middleware.",
		JSONPath:    "/swagger.json",
		UIPath:      "/swagger",
	}))

	// ── Routes ───────────────────────────────────────────────────────────────

	// GET /users — list users
	router.Handle(breeze.GET, "/users", listUsers,
		middleware.DocGET("/users", swagger.RouteDoc{
			Title:       "List users",
			Tags:        []string{"Users"},
			Description: "Returns a paginated list of users.",
			Input: []swagger.InputGroup{
				{
					Type:        swagger.InputQuery,
					Fields:      UserQueryParams{},
					Description: "Pagination and sorting options",
				},
			},
			Output:            UserListResponse{},
			OutputStatus:      200,
			OutputDescription: "Paginated user list",
		}),
	)

	// POST /users — create user
	router.Handle(breeze.POST, "/users", createUser,
		middleware.DocPOST("/users", swagger.RouteDoc{
			Title: "Create user",
			Tags:  []string{"Users"},
			Input: []swagger.InputGroup{
				{
					Type:     swagger.InputBody,
					Fields:   CreateUserRequest{},
					Required: true,
				},
			},
			Output:       UserResponse{},
			OutputStatus: 201,
		}),
	)

	// GET /users/:id — get single user
	router.Handle(breeze.GET, "/users/:id", getUser,
		middleware.DocGET("/users/:id", swagger.RouteDoc{
			Title: "Get user by ID",
			Tags:  []string{"Users"},
			Input: []swagger.InputGroup{
				{
					Type:   swagger.InputParams,
					Fields: UserPathParams{},
				},
			},
			Output: UserResponse{},
		}),
	)

	// DELETE /users/:id — delete user
	router.Handle(breeze.DELETE, "/users/:id", deleteUser,
		middleware.DocDELETE("/users/:id", swagger.RouteDoc{
			Title:        "Delete user",
			Tags:         []string{"Users"},
			Input: []swagger.InputGroup{
				{Type: swagger.InputParams, Fields: UserPathParams{}},
			},
			Output:            struct{}{},
			OutputStatus:      204,
			OutputDescription: "User deleted",
		}),
	)

	app := breeze.New(router, breeze.NewWorkerPool(runtime.NumCPU()))
	app.Run(3000, true)
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func listUsers(ctx *breeze.Context) {
	ctx.JSON(UserListResponse{
		Users: []UserResponse{
			{ID: "1", Name: "Alice", Email: "alice@example.com"},
		},
		Total: 1,
	})
}

func createUser(ctx *breeze.Context) {
	ctx.Status(201)
	ctx.JSON(UserResponse{ID: "2", Name: "Bob", Email: "bob@example.com"})
}

func getUser(ctx *breeze.Context) {
	ctx.JSON(UserResponse{
		ID:    ctx.GetParam("id"),
		Name:  "Alice",
		Email: "alice@example.com",
	})
}

func deleteUser(ctx *breeze.Context) {
	ctx.Status(204)
}
