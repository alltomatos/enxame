package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	Phone        string
	Nickname     string
	Role         string
	CreatedAt    time.Time
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser cadastrar um novo usuário com lógica Gênesis (Primeiro = Owner)
func (r *UserRepository) CreateUser(ctx context.Context, u *User) (*User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lógica Gênesis: Verificar se a tabela está vazia dentro da transação
	var count int
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	if count == 0 {
		u.Role = "owner"
	} else {
		u.Role = "member"
	}

	query := `
		INSERT INTO users (email, password_hash, full_name, phone, nickname, role)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err = tx.QueryRow(ctx, query, u.Email, u.PasswordHash, u.FullName, u.Phone, u.Nickname, u.Role).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return u, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, phone, nickname, role, created_at
		FROM users WHERE email = $1
	`
	u := &User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.Nickname, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, phone, nickname, role, created_at
		FROM users WHERE id = $1
	`
	u := &User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.Nickname, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, email, full_name, phone, nickname, role, created_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Nickname, &u.Role, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}

	return users, total, nil
}

func (r *UserRepository) UpdateUserRole(ctx context.Context, userID, newRole string) error {
	query := `UPDATE users SET role = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, newRole)
	return err
}

func (r *UserRepository) GetTotalUsers(ctx context.Context) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&total)
	return total, err
}
