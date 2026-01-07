package local

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EntityType string
type AttributeType string

const (
	EntityTypeUser    EntityType = "user"
	EntityTypeService EntityType = "service"

	AttributeTypeRequest  AttributeType = "request"
	AttributeTypeResponse AttributeType = "response"
)

type User struct {
	ID        int64
	Username  string
	Password  *string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Service struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserService struct {
	ID        int64
	UserID    int64
	ServiceID int64
	Priority  int
	CreatedAt time.Time
}

type Attribute struct {
	ID             int64
	EntityType     EntityType
	EntityID       int64
	AttributeType  AttributeType
	AttributeName  string
	AttributeValue string
	Op             string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled);

	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_services_name ON services(name);

	CREATE TABLE IF NOT EXISTS user_services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		service_id INTEGER NOT NULL,
		priority INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE,
		UNIQUE(user_id, service_id)
	);

	CREATE INDEX IF NOT EXISTS idx_user_services_user ON user_services(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_services_service ON user_services(service_id);
	CREATE INDEX IF NOT EXISTS idx_user_services_priority ON user_services(user_id, priority);

	CREATE TABLE IF NOT EXISTS attributes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_type TEXT NOT NULL CHECK(entity_type IN ('user', 'service')),
		entity_id INTEGER NOT NULL,
		attribute_type TEXT NOT NULL CHECK(attribute_type IN ('request', 'response')),
		attribute_name TEXT NOT NULL,
		attribute_value TEXT NOT NULL,
		op TEXT DEFAULT '=',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(entity_type, entity_id, attribute_type, attribute_name)
	);

	CREATE INDEX IF NOT EXISTS idx_attributes_entity ON attributes(entity_type, entity_id);
	CREATE INDEX IF NOT EXISTS idx_attributes_lookup ON attributes(entity_type, entity_id, attribute_type);
	CREATE INDEX IF NOT EXISTS idx_attributes_name ON attributes(attribute_name);
	`

	_, err := db.Exec(schema)
	return err
}

func getUserByUsername(db *sql.DB, username string) (*User, error) {
	query := `SELECT id, username, password, enabled, created_at, updated_at
	          FROM users WHERE username = ?`

	var user User
	var password sql.NullString

	err := db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&password,
		&user.Enabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %s", username)
		}
		return nil, err
	}

	if password.Valid {
		user.Password = &password.String
	}

	return &user, nil
}

func GetUserByID(db *sql.DB, userID int64) (*User, error) {
	query := `SELECT id, username, password, enabled, created_at, updated_at
	          FROM users WHERE id = ?`

	var user User
	var password sql.NullString

	err := db.QueryRow(query, userID).Scan(
		&user.ID,
		&user.Username,
		&password,
		&user.Enabled,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, err
	}

	if password.Valid {
		user.Password = &password.String
	}

	return &user, nil
}

func loadMergedAttributes(db *sql.DB, userID int64) (map[string]string, error) {
	result := make(map[string]string)

	serviceQuery := `
		SELECT a.attribute_name, a.attribute_value
		FROM attributes a
		INNER JOIN user_services us ON a.entity_id = us.service_id
		WHERE us.user_id = ?
		  AND a.entity_type = ?
		  AND a.attribute_type = ?
		ORDER BY us.priority ASC, us.id ASC
	`

	rows, err := db.Query(serviceQuery, userID, EntityTypeService, AttributeTypeResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to load service attributes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		if _, exists := result[name]; !exists {
			result[name] = value
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	userQuery := `
		SELECT attribute_name, attribute_value
		FROM attributes
		WHERE entity_type = ?
		  AND entity_id = ?
		  AND attribute_type = ?
	`

	rows, err = db.Query(userQuery, EntityTypeUser, userID, AttributeTypeResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to load user attributes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		result[name] = value
	}

	return result, rows.Err()
}

func CreateUser(db *sql.DB, username string, password *string, enabled bool) (int64, error) {
	query := `INSERT INTO users (username, password, enabled) VALUES (?, ?, ?)`
	result, err := db.Exec(query, username, password, enabled)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	return result.LastInsertId()
}

func UpdateUserPasswordByID(db *sql.DB, userID int64, password *string) error {
	query := `UPDATE users SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := db.Exec(query, password, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user not found with ID: %d", userID)
	}

	return nil
}

func UpdateUserEnabledByID(db *sql.DB, userID int64, enabled bool) error {
	query := `UPDATE users SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := db.Exec(query, enabled, userID)
	if err != nil {
		return fmt.Errorf("failed to update enabled status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user not found with ID: %d", userID)
	}

	return nil
}

func DeleteUserByID(db *sql.DB, userID int64) error {
	query := `DELETE FROM users WHERE id = ?`
	result, err := db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user not found with ID: %s", userID)
	}

	return nil
}

func ListUsers(db *sql.DB) ([]User, error) {
	query := `SELECT id, username, password, enabled, created_at, updated_at FROM users ORDER BY username`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var password sql.NullString

		if err := rows.Scan(&user.ID, &user.Username, &password, &user.Enabled, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}

		if password.Valid {
			user.Password = &password.String
		}

		users = append(users, user)
	}

	return users, rows.Err()
}

func CreateService(db *sql.DB, name, description string) (int64, error) {
	query := `INSERT INTO services (name, description) VALUES (?, ?)`
	result, err := db.Exec(query, name, description)
	if err != nil {
		return 0, fmt.Errorf("failed to create service: %w", err)
	}
	return result.LastInsertId()
}

func GetServiceByName(db *sql.DB, name string) (*Service, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM services WHERE name = ?`

	var service Service
	err := db.QueryRow(query, name).Scan(
		&service.ID,
		&service.Name,
		&service.Description,
		&service.CreatedAt,
		&service.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("service not found: %s", name)
		}
		return nil, err
	}

	return &service, nil
}

func GetServiceByID(db *sql.DB, id int64) (*Service, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM services WHERE id = ?`

	var service Service
	err := db.QueryRow(query, id).Scan(
		&service.ID,
		&service.Name,
		&service.Description,
		&service.CreatedAt,
		&service.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("service not found: %d", id)
		}
		return nil, err
	}

	return &service, nil
}

func DeleteService(db *sql.DB, name string) error {
	query := `DELETE FROM services WHERE name = ?`
	result, err := db.Exec(query, name)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("service not found: %s", name)
	}

	return nil
}

func ListServices(db *sql.DB) ([]Service, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM services ORDER BY name`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var service Service
		if err := rows.Scan(&service.ID, &service.Name, &service.Description, &service.CreatedAt, &service.UpdatedAt); err != nil {
			return nil, err
		}
		services = append(services, service)
	}

	return services, rows.Err()
}

func AssignUserServiceByID(db *sql.DB, userID int64, serviceName string, priority int) error {
	service, err := GetServiceByName(db, serviceName)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO user_services (user_id, service_id, priority)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, service_id) DO UPDATE SET priority = excluded.priority
	`

	_, err = db.Exec(query, userID, service.ID, priority)
	if err != nil {
		return fmt.Errorf("failed to assign service to user: %w", err)
	}

	return nil
}

func GetUserServices(db *sql.DB, username string) ([]UserService, error) {
	user, err := getUserByUsername(db, username)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, service_id, priority, created_at
		FROM user_services
		WHERE user_id = ?
		ORDER BY priority ASC
	`

	rows, err := db.Query(query, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user services: %w", err)
	}
	defer rows.Close()

	var userServices []UserService
	for rows.Next() {
		var us UserService
		if err := rows.Scan(&us.ID, &us.UserID, &us.ServiceID, &us.Priority, &us.CreatedAt); err != nil {
			return nil, err
		}
		userServices = append(userServices, us)
	}

	return userServices, rows.Err()
}

func SetAttributeByID(db *sql.DB, entityType EntityType, entityID int64, attributeType AttributeType, attributeName string, attributeValue string, op string) error {
	query := `
		INSERT INTO attributes (entity_type, entity_id, attribute_type, attribute_name, attribute_value, op)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, entity_id, attribute_type, attribute_name)
		DO UPDATE SET attribute_value = excluded.attribute_value, op = excluded.op, updated_at = CURRENT_TIMESTAMP
	`

	_, err := db.Exec(query, entityType, entityID, attributeType, attributeName, attributeValue, op)
	if err != nil {
		return fmt.Errorf("failed to set attribute: %w", err)
	}

	return nil
}

func GetAttributes(db *sql.DB, entityType EntityType, entityName string, attributeType AttributeType) ([]Attribute, error) {
	var entityID int64

	if entityType == EntityTypeUser {
		user, err := getUserByUsername(db, entityName)
		if err != nil {
			return nil, err
		}
		entityID = user.ID
	} else if entityType == EntityTypeService {
		service, err := GetServiceByName(db, entityName)
		if err != nil {
			return nil, err
		}
		entityID = service.ID
	} else {
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}

	query := `
		SELECT id, entity_type, entity_id, attribute_type, attribute_name, attribute_value, op, created_at, updated_at
		FROM attributes
		WHERE entity_type = ? AND entity_id = ? AND attribute_type = ?
		ORDER BY attribute_name
	`

	rows, err := db.Query(query, entityType, entityID, attributeType)
	if err != nil {
		return nil, fmt.Errorf("failed to get attributes: %w", err)
	}
	defer rows.Close()

	var attributes []Attribute
	for rows.Next() {
		var attr Attribute
		var entityTypeStr, attributeTypeStr string

		if err := rows.Scan(
			&attr.ID,
			&entityTypeStr,
			&attr.EntityID,
			&attributeTypeStr,
			&attr.AttributeName,
			&attr.AttributeValue,
			&attr.Op,
			&attr.CreatedAt,
			&attr.UpdatedAt,
		); err != nil {
			return nil, err
		}

		attr.EntityType = EntityType(entityTypeStr)
		attr.AttributeType = AttributeType(attributeTypeStr)
		attributes = append(attributes, attr)
	}

	return attributes, rows.Err()
}
