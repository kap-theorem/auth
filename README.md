# Auth Service

A comprehensive Authentication as a Service (AaaS) built with Go and gRPC. This service provides user authentication, session management, and client registration capabilities.

## Features

- **User Management**: Registration, login, and profile management
- **JWT Authentication**: Secure token-based authentication
- **Session Management**: Refresh tokens and session handling
- **Client Management**: Multi-client support with client registration
- **Security**: Password hashing, token validation, and session expiry
- **Database**: MySQL with GORM ORM
- **Health Checks**: Built-in health monitoring
- **Cleanup Service**: Automatic expired session cleanup

## Architecture

```
├── cmd/server/          # Application entry point
├── internal/database/   # Database connection and setup
├── pkg/
│   ├── models/         # Data models
│   ├── repository/     # Data access layer
│   ├── service/        # Business logic
│   └── utils/          # Utility functions
├── proto/auth/v1/      # Protocol buffer definitions
└── .env.example        # Environment variables template
```

## Prerequisites

- Go 1.21+
- MySQL 8.0+
- Protocol Buffers compiler (protoc)
- gRPC tools

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd auth
```

2. Install dependencies:
```bash
go mod tidy
```

3. Set up environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

4. Generate protobuf files (if modified):
```bash
protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto
```

## Configuration

Create a `.env` file with the following variables:

```env
# Database Configuration
DB_CONNECTION_STRING=user:password@tcp(localhost:3306)/authdb?charset=utf8mb4&parseTime=True&loc=Local

# JWT Configuration
JWT_SECRET=your-super-secure-jwt-secret-key-here-make-it-long-and-random

# Server Configuration
SERVER_PORT=8080
```

## Database Setup

The service will automatically create the required tables on startup. Ensure your MySQL database is running and accessible.

### Tables Created:
- `users`: User information and credentials
- `clients`: Registered client applications
- `sessions`: User sessions and refresh tokens

## Running the Service

```bash
go run cmd/server/main.go
```

The server will start on port 8080 (or the port specified in your environment).

## API Documentation

### gRPC Service: AuthService

#### 1. Health Check
```protobuf
rpc HealthCheck(google.protobuf.Empty) returns (HealthCheckResponse);
```
**Purpose**: Check service health and availability.

**Response**:
- `status`: Service status (SERVING, NOT_SERVING, SERVICE_UNKNOWN)
- `message`: Status message
- `details`: Additional service information

#### 2. Register Client
```protobuf
rpc RegisterClient(RegisterClientRequest) returns (RegisterClientResponse);
```
**Purpose**: Register a new client application.

**Request**:
- `client_name`: Name of the client application

**Response**:
- `success`: Operation success status
- `message`: Response message
- `client_id`: Generated client ID (UUID)
- `client_secret`: Generated client secret

#### 3. Register User
```protobuf
rpc RegisterUser(RegisterUserRequest) returns (RegisterUserResponse);
```
**Purpose**: Register a new user account.

**Request**:
- `username`: User's display name
- `email`: User's email address (must be unique)
- `password`: User's password (minimum 8 characters)
- `client_id`: Client ID the user belongs to

**Response**:
- `success`: Operation success status
- `message`: Response message
- `user_id`: Generated user ID (UUID)

#### 4. Login User
```protobuf
rpc LoginUser(LoginUserRequest) returns (LoginUserResponse);
```
**Purpose**: Authenticate user and create session.

**Request**:
- `email`: User's email address
- `password`: User's password
- `client_id`: Client ID
- `user_agent`: Optional user agent string

**Response**:
- `success`: Operation success status
- `message`: Response message
- `access_token`: JWT access token (24-hour expiry)
- `refresh_token`: Refresh token (7-day expiry)
- `expires_at`: Token expiration timestamp
- `user`: User profile information

#### 5. Validate Token
```protobuf
rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
```
**Purpose**: Validate JWT access token.

**Request**:
- `access_token`: JWT token to validate

**Response**:
- `valid`: Token validity status
- `message`: Validation message
- `user_id`: User ID from token (if valid)
- `expires_at`: Token expiration timestamp

#### 6. Refresh Token
```protobuf
rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
```
**Purpose**: Refresh access token using refresh token.

**Request**:
- `refresh_token`: Valid refresh token
- `client_id`: Client ID

**Response**:
- `success`: Operation success status
- `message`: Response message
- `access_token`: New JWT access token
- `refresh_token`: New refresh token
- `expires_at`: New token expiration timestamp

#### 7. Logout User
```protobuf
rpc LogoutUser(LogoutUserRequest) returns (LogoutUserResponse);
```
**Purpose**: Logout user and invalidate session.

**Request**:
- `refresh_token`: Refresh token to invalidate

**Response**:
- `success`: Operation success status
- `message`: Response message

#### 8. Get User Profile
```protobuf
rpc GetUserProfile(GetUserProfileRequest) returns (GetUserProfileResponse);
```
**Purpose**: Retrieve user profile information.

**Request**:
- `access_token`: Valid JWT access token

**Response**:
- `success`: Operation success status
- `message`: Response message
- `user`: User profile information

## Usage Examples

### Testing with grpcurl

1. **Health Check**:
```bash
grpcurl -plaintext localhost:8080 auth.v1.AuthService/HealthCheck
```

2. **Register Client**:
```bash
grpcurl -plaintext -d '{"client_name": "My App"}' localhost:8080 auth.v1.AuthService/RegisterClient
```

3. **Register User**:
```bash
grpcurl -plaintext -d '{"username": "john_doe", "email": "john@example.com", "password": "password123", "client_id": "YOUR_CLIENT_ID"}' localhost:8080 auth.v1.AuthService/RegisterUser
```

4. **Login User**:
```bash
grpcurl -plaintext -d '{"email": "john@example.com", "password": "password123", "client_id": "YOUR_CLIENT_ID", "user_agent": "grpcurl"}' localhost:8080 auth.v1.AuthService/LoginUser
```

### Integration Flow

1. **Client Registration**: Register your application to get `client_id` and `client_secret`
2. **User Registration**: Users register with their credentials and your `client_id`
3. **Authentication**: Users login to receive `access_token` and `refresh_token`
4. **API Calls**: Include `access_token` in requests to protected endpoints
5. **Token Refresh**: Use `refresh_token` to get new tokens when access token expires
6. **Logout**: Invalidate session when user logs out

## Security Features

- **Password Hashing**: bcrypt with salt
- **JWT Tokens**: HMAC-SHA256 signed tokens
- **Session Management**: Secure refresh token rotation
- **Client Validation**: Multi-tenant support with client isolation
- **Input Validation**: Email format, password strength, required fields
- **Automatic Cleanup**: Expired sessions are cleaned up hourly

## Error Handling

The service provides detailed error messages for:
- Invalid credentials
- Missing required fields
- Token validation failures
- Client authentication issues
- Database connectivity problems

## Monitoring

- **Logging**: Comprehensive request/response logging
- **Health Checks**: Built-in health endpoint
- **Metrics**: Request duration and method tracking via interceptors

## Development

### Adding New Endpoints

1. Update `proto/auth/v1/auth.proto`
2. Regenerate protobuf files: `protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto`
3. Implement method in `pkg/service/auth_service.go`
4. Add repository methods if needed in `pkg/repository/auth_repository.go`

### Database Migrations

The service uses GORM AutoMigrate. To modify schemas:
1. Update models in `pkg/models/model.go`
2. Restart the service to apply changes

## License

[License information]

## Contributing

[Contributing guidelines]
