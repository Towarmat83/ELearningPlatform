package fake

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// UserRepository is an in-memory repository.UserRepository for tests.
//
// Cross-aggregate projections (ListForAdmin/GetForAdmin/Leaderboard) only
// have access to seeded user rows: enrollment/lesson/module-progress counts
// they'd normally join against always report as zero here.
type UserRepository struct {
	mu    sync.Mutex
	users []models.User

	// Err, when set, is returned by every method call (simulates a DB failure).
	Err error
	// CreateErr, when set, is returned by Create only, on top of Err — kept
	// separate so tests can simulate ExistsByEmailOrUsername succeeding right
	// before Create fails.
	CreateErr error
	// UpdatePasswordHashErr, when set, is returned by UpdatePasswordHash only,
	// on top of Err — kept separate so tests can simulate GetPasswordHash
	// succeeding right before UpdatePasswordHash fails.
	UpdatePasswordHashErr error
	// RefreshSSOProfileErr, when set, is returned by RefreshSSOProfile only,
	// on top of Err — kept separate so tests can simulate FindByProviderIdentity
	// succeeding right before RefreshSSOProfile fails.
	RefreshSSOProfileErr error
	// LinkProviderIdentityErr, when set, is returned by LinkProviderIdentity
	// only, on top of Err — kept separate so tests can simulate FindByEmail
	// succeeding right before LinkProviderIdentity fails.
	LinkProviderIdentityErr error
}

// NewUserRepository builds a fake UserRepository seeded with users.
func NewUserRepository(seed ...models.User) *UserRepository {
	return &UserRepository{users: append([]models.User{}, seed...)}
}

func (f *UserRepository) findIndexByID(id uuid.UUID) int {
	for i := range f.users {
		if f.users[i].ID == id {
			return i
		}
	}

	return -1
}

func (f *UserRepository) ExistsByEmailOrUsername(_ context.Context, email, username string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for _, u := range f.users {
		if u.Email == email || u.Username == username {
			return true, nil
		}
	}

	return false, nil
}

func (f *UserRepository) Create(_ context.Context, u *models.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	if f.CreateErr != nil {
		return f.CreateErr
	}

	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}

	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}

	u.UpdatedAt = now

	f.users = append(f.users, *u)

	return nil
}

func (f *UserRepository) FindByEmailActive(_ context.Context, email string) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	for _, u := range f.users {
		if u.Email == email && u.IsActive {
			cp := u

			return &cp, nil
		}
	}

	return nil, repository.ErrUserNotFound
}

func (f *UserRepository) FindByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	cp := f.users[i]

	return &cp, nil
}

func (f *UserRepository) ExistsUsernameExcluding(_ context.Context, username string, excludeID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for _, u := range f.users {
		if u.Username == username && u.ID != excludeID {
			return true, nil
		}
	}

	return false, nil
}

func (f *UserRepository) UpdateProfile(_ context.Context, id uuid.UUID, username, bio, avatarURL *string) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	if username != nil {
		f.users[i].Username = *username
	}

	if bio != nil {
		f.users[i].Bio = bio
	}

	if avatarURL != nil {
		f.users[i].AvatarURL = avatarURL
	}

	f.users[i].UpdatedAt = time.Now()

	cp := f.users[i]

	return &cp, nil
}

func (f *UserRepository) GetPasswordHash(_ context.Context, id uuid.UUID) (*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	return f.users[i].PasswordHash, nil
}

func (f *UserRepository) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	if f.UpdatePasswordHashErr != nil {
		return f.UpdatePasswordHashErr
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return repository.ErrUserNotFound
	}

	f.users[i].PasswordHash = &hash
	f.users[i].UpdatedAt = time.Now()

	return nil
}

func (f *UserRepository) ExistsByUsername(_ context.Context, username string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return false, f.Err
	}

	for _, u := range f.users {
		if u.Username == username {
			return true, nil
		}
	}

	return false, nil
}

func (f *UserRepository) FindByProviderIdentity(_ context.Context, provider, providerUserID string) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	for _, u := range f.users {
		if u.AuthProvider == provider && u.ProviderUserID != nil && *u.ProviderUserID == providerUserID {
			cp := u

			return &cp, nil
		}
	}

	return nil, repository.ErrUserNotFound
}

func (f *UserRepository) FindByEmail(_ context.Context, email string) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	for _, u := range f.users {
		if u.Email == email {
			cp := u

			return &cp, nil
		}
	}

	return nil, repository.ErrUserNotFound
}

func (f *UserRepository) RefreshSSOProfile(_ context.Context, id uuid.UUID, avatarURL, bio *string) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	if f.RefreshSSOProfileErr != nil {
		return nil, f.RefreshSSOProfileErr
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	if avatarURL != nil {
		f.users[i].AvatarURL = avatarURL
	}

	if f.users[i].Bio == nil || *f.users[i].Bio == "" {
		if bio != nil {
			f.users[i].Bio = bio
		}
	}

	f.users[i].UpdatedAt = time.Now()

	cp := f.users[i]

	return &cp, nil
}

func (f *UserRepository) LinkProviderIdentity(
	_ context.Context, id uuid.UUID, provider, providerUserID string, avatarURL, bio *string,
) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	if f.LinkProviderIdentityErr != nil {
		return nil, f.LinkProviderIdentityErr
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	f.users[i].AuthProvider = provider
	f.users[i].ProviderUserID = &providerUserID

	if avatarURL != nil {
		f.users[i].AvatarURL = avatarURL
	}

	if f.users[i].Bio == nil || *f.users[i].Bio == "" {
		if bio != nil {
			f.users[i].Bio = bio
		}
	}

	f.users[i].UpdatedAt = time.Now()

	cp := f.users[i]

	return &cp, nil
}

func (f *UserRepository) CountByRole(_ context.Context, role string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return 0, f.Err
	}

	var count int64

	for _, u := range f.users {
		if u.Role == role {
			count++
		}
	}

	return count, nil
}

func (f *UserRepository) ListAuthProviders(_ context.Context) ([]repository.AuthProviderCount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	counts := map[string]int64{}
	for _, u := range f.users {
		counts[u.AuthProvider]++
	}

	list := make([]repository.AuthProviderCount, 0, len(counts))
	for provider, count := range counts {
		list = append(list, repository.AuthProviderCount{Provider: provider, Count: count})
	}

	return list, nil
}

func (f *UserRepository) ListForAdmin(_ context.Context, provider string) ([]repository.AdminUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]repository.AdminUserRow, 0, len(f.users))

	for _, u := range f.users {
		if provider != "" && u.AuthProvider != provider {
			continue
		}

		list = append(list, adminUserRowFromModel(u))
	}

	return list, nil
}

func (f *UserRepository) GetForAdmin(_ context.Context, id uuid.UUID) (*repository.AdminUserDetailRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	row := adminUserRowFromModel(f.users[i])

	return &repository.AdminUserDetailRow{
		ID: row.ID, Username: row.Username, Email: row.Email, Role: row.Role, IsActive: row.IsActive,
		AvatarURL: row.AvatarURL, Bio: row.Bio, AuthProvider: row.AuthProvider, CreatedAt: row.CreatedAt,
		EnrolledCourses: row.EnrolledCourses,
	}, nil
}

func adminUserRowFromModel(u models.User) repository.AdminUserRow {
	return repository.AdminUserRow{
		ID: u.ID.String(), Username: u.Username, Email: u.Email, Role: u.Role, IsActive: u.IsActive,
		AvatarURL: u.AvatarURL, Bio: u.Bio, AuthProvider: u.AuthProvider, CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}

func (f *UserRepository) UpdateAdminFields(
	_ context.Context, id uuid.UUID, username, bio, avatarURL *string, isActive *bool, role *string,
) (*models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil, repository.ErrUserNotFound
	}

	if username != nil {
		f.users[i].Username = *username
	}

	if bio != nil {
		f.users[i].Bio = bio
	}

	if avatarURL != nil {
		f.users[i].AvatarURL = avatarURL
	}

	if isActive != nil {
		f.users[i].IsActive = *isActive
	}

	if role != nil {
		f.users[i].Role = *role
	}

	f.users[i].UpdatedAt = time.Now()

	cp := f.users[i]

	return &cp, nil
}

func (f *UserRepository) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return f.Err
	}

	i := f.findIndexByID(id)
	if i < 0 {
		return nil
	}

	f.users = append(f.users[:i], f.users[i+1:]...)

	return nil
}

func (f *UserRepository) Search(_ context.Context, query string) ([]repository.UserSearchRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	q := strings.ToLower(query)

	results := make([]repository.UserSearchRow, 0)

	for _, u := range f.users {
		if strings.Contains(strings.ToLower(u.Username), q) || strings.Contains(strings.ToLower(u.Email), q) {
			results = append(results, repository.UserSearchRow{ID: u.ID.String(), Username: u.Username, Email: u.Email})
		}
	}

	return results, nil
}

func (f *UserRepository) Leaderboard(_ context.Context) ([]repository.LeaderboardRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	list := make([]repository.LeaderboardRow, 0)

	for _, u := range f.users {
		if u.Role != "student" || !u.IsActive {
			continue
		}

		list = append(list, repository.LeaderboardRow{
			ID: u.ID.String(), Username: u.Username, Email: u.Email, AvatarURL: u.AvatarURL,
		})
	}

	return list, nil
}

func (f *UserRepository) CreateAdminIfAbsent(_ context.Context, username, email, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if u.Email == email {
			return nil
		}
	}

	f.users = append(f.users, models.User{
		ID: uuid.New(), Username: username, Email: email, PasswordHash: &hash, Role: "admin",
		IsActive: true, AuthProvider: "local", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	return nil
}

func (f *UserRepository) UpsertAdminPassword(_ context.Context, username, email, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, u := range f.users {
		if u.Email == email {
			f.users[i].PasswordHash = &hash
			f.users[i].UpdatedAt = time.Now()

			return nil
		}
	}

	f.users = append(f.users, models.User{
		ID: uuid.New(), Username: username, Email: email, PasswordHash: &hash, Role: "admin",
		IsActive: true, AuthProvider: "local", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	return nil
}

func (f *UserRepository) UpsertMockStudent(_ context.Context, username, email, hash string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, u := range f.users {
		if u.Email == email {
			f.users[i].Username = username

			return f.users[i].ID, nil
		}
	}

	id := uuid.New()
	f.users = append(f.users, models.User{
		ID: id, Username: username, Email: email, PasswordHash: &hash, Role: "student",
		IsActive: true, AuthProvider: "local", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	return id, nil
}
