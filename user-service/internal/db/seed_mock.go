package db

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// mockStudentPassword is the cleartext password used for all mock students.
// Hash of "Student@1234" (cost 12).
const mockStudentPassword = "Student@1234"

type mockUser struct {
	username string
	email    string
}

var mockUsers = []mockUser{
	{"amandine.dupont", "amandine.dupont@example.com"},
	{"theo.martin", "theo.martin@example.com"},
	{"lena.bernard", "lena.bernard@example.com"},
	{"karim.benali", "karim.benali@example.com"},
	{"sofia.iglesias", "sofia.iglesias@example.com"},
	{"lucas.petit", "lucas.petit@example.com"},
	{"yasmine.toure", "yasmine.toure@example.com"},
	{"maxime.leroy", "maxime.leroy@example.com"},
	{"ines.fontaine", "ines.fontaine@example.com"},
	{"rayan.cherif", "rayan.cherif@example.com"},
	{"julie.moreau", "julie.moreau@example.com"},
	{"baptiste.simon", "baptiste.simon@example.com"},
	{"chiara.russo", "chiara.russo@example.com"},
	{"adrien.lambert", "adrien.lambert@example.com"},
	{"fatou.diallo", "fatou.diallo@example.com"},
	{"hugo.renard", "hugo.renard@example.com"},
	{"camille.vidal", "camille.vidal@example.com"},
	{"omar.ndiaye", "omar.ndiaye@example.com"},
	{"alice.garnier", "alice.garnier@example.com"},
	{"mehdi.boukhari", "mehdi.boukhari@example.com"},
	{"pauline.aubert", "pauline.aubert@example.com"},
	{"tom.leclerc", "tom.leclerc@example.com"},
	{"nadia.khelifi", "nadia.khelifi@example.com"},
	{"vincent.perez", "vincent.perez@example.com"},
	{"elisa.nguyen", "elisa.nguyen@example.com"},
}

// mockEnrollments maps username → list of course slugs.
var mockEnrollments = map[string][]string{
	"amandine.dupont": {"linux-intro", "docker-fundamentals", "python-basics"},
	"theo.martin":     {"linux-intro", "kubernetes-basics", "docker-fundamentals"},
	"lena.bernard":    {"python-basics"},
	"karim.benali":    {"linux-intro", "networking-essentials", "cybersecurity-intro"},
	"sofia.iglesias":  {"python-basics", "linux-intro"},
	"lucas.petit":     {"linux-intro", "docker-fundamentals", "git-advanced", "kubernetes-basics"},
	"yasmine.toure":   {"networking-essentials", "cybersecurity-intro"},
	"maxime.leroy":    {"linux-intro", "python-basics", "docker-fundamentals"},
	"ines.fontaine":   {"git-advanced", "docker-fundamentals"},
	"rayan.cherif":    {"linux-intro", "networking-essentials"},
	"julie.moreau":    {"python-basics", "cybersecurity-intro"},
	"baptiste.simon":  {"linux-intro", "kubernetes-basics", "networking-essentials", "git-advanced"},
	"chiara.russo":    {"python-basics", "linux-intro"},
	"adrien.lambert":  {"docker-fundamentals", "kubernetes-basics", "cybersecurity-intro"},
	"fatou.diallo":    {"linux-intro", "python-basics", "networking-essentials"},
	"hugo.renard":     {"linux-intro", "docker-fundamentals"},
	"camille.vidal":   {"python-basics", "git-advanced"},
	"omar.ndiaye":     {"linux-intro", "networking-essentials", "cybersecurity-intro"},
	"alice.garnier":   {"python-basics", "docker-fundamentals", "kubernetes-basics"},
	"mehdi.boukhari":  {"linux-intro", "cybersecurity-intro"},
	"pauline.aubert":  {"python-basics"},
	"tom.leclerc":     {"linux-intro", "docker-fundamentals", "git-advanced"},
	"nadia.khelifi":   {"networking-essentials", "cybersecurity-intro", "linux-intro"},
	"vincent.perez":   {"kubernetes-basics", "docker-fundamentals"},
	"elisa.nguyen":    {"python-basics", "linux-intro", "git-advanced"},
}

// SeedMockData inserts mock student users and their enrollments.
// It is idempotent: existing rows are skipped via ON CONFLICT DO NOTHING.
// Activate with SEED_MOCK_DATA=true at startup.
func SeedMockData(ctx context.Context, pool *pgxpool.Pool) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(mockStudentPassword), bcryptCost)
	if err != nil {
		return err
	}

	passwordHash := string(hash)

	for _, u := range mockUsers {
		var userID string

		err := pool.QueryRow(ctx, `
			INSERT INTO users (username, email, password_hash, role)
			VALUES ($1, $2, $3, 'student')
			ON CONFLICT (email) DO UPDATE SET username = EXCLUDED.username
			RETURNING id`,
			u.username, u.email, passwordHash,
		).Scan(&userID)
		if err != nil {
			slog.Error("mock seed: failed to upsert user", "username", u.username, "err", err)

			continue
		}

		for _, slug := range mockEnrollments[u.username] {
			_, err := pool.Exec(ctx, `
				INSERT INTO enrollments (user_id, course_slug)
				VALUES ($1, $2)
				ON CONFLICT DO NOTHING`,
				userID, slug,
			)
			if err != nil {
				slog.Error("mock seed: failed to enroll user", "username", u.username, "course", slug, "err", err)
			}
		}
	}

	slog.Info("mock seed: inserted students and enrollments", "count", len(mockUsers))

	return nil
}
