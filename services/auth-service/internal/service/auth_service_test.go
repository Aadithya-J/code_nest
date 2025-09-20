package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const testSchema = "auth_test"

// AuthServiceTestSuite is the test suite for the AuthService.
type AuthServiceTestSuite struct {
	suite.Suite
	db      *gorm.DB
	service *AuthService
	repo    *repository.UserRepo
}

// SetupSuite runs once before the entire test suite.
func (s *AuthServiceTestSuite) SetupSuite() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://user:password@localhost:5433/code_nest_db?sslmode=disable"
		log.Println("DATABASE_URL not set, using default for testing")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database for testing: %v", err)
	}
	s.db = db

	// Create a dedicated schema for testing to isolate test data.
	if err := s.db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", testSchema)).Error; err != nil {
		log.Fatalf("failed to create test schema: %v", err)
	}

	// Re-initialize GORM with the test schema in the search path.
	testDsn := fmt.Sprintf("%s&search_path=%s", dsn, testSchema)
	testDb, err := gorm.Open(postgres.Open(testDsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to test database with schema: %v", err)
	}
	s.db = testDb

	// Run migrations in the test schema.
	if err := s.db.AutoMigrate(&repository.User{}); err != nil {
		log.Fatalf("failed to migrate test database: %v", err)
	}

	s.repo = repository.NewUserRepo(s.db)
	s.service, err = NewAuthService(s.repo, &oauth2.Config{}) // Using a dummy oauth config for now.
	if err != nil {
		log.Fatalf("failed to create auth service for testing: %v", err)
	}
}

// TearDownSuite runs once after the entire test suite is finished.
func (s *AuthServiceTestSuite) TearDownSuite() {
	if err := s.db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", testSchema)).Error; err != nil {
		log.Printf("failed to drop test schema: %v", err)
	}
}

// SetupTest runs before each test to ensure a clean state.
func (s *AuthServiceTestSuite) SetupTest() {
	// Clear the users table before each test.
	s.db.Exec("DELETE FROM users")
}

// TestSignup tests the user registration process.
func (s *AuthServiceTestSuite) TestSignup() {
	ctx := context.Background()
	req := &proto.SignupRequest{
		Email:    "test@example.com",
		Password: "password123",
	}

	resp, err := s.service.Signup(ctx, req)

	// Assertions
	s.NoError(err, "Signup should not return an error")
	s.NotNil(resp, "Response should not be nil")
	s.NotEmpty(resp.Token, "Token should not be empty")

	// Verify user in the database
	user, dbErr := s.repo.FindByEmail("test@example.com")
	s.NoError(dbErr, "Should be able to find user in DB after signup")
	s.NotNil(user, "User should exist in the database")
	s.Equal("test@example.com", user.Email, "User email should match")
	s.NotEmpty(user.PasswordHash, "Password hash should be stored")
}

// TestAuthService is the entry point for running the test suite.
func TestAuthService(t *testing.T) {
	suite.Run(t, new(AuthServiceTestSuite))
}
