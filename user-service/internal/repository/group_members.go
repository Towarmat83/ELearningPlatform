package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm/clause"

	"github.com/genesary/pupitre/user-service/internal/models"
)

// ListMembers returns every user belonging to the group identified by groupID.
func (r *gormGroupRepository) ListMembers(ctx context.Context, groupID string) ([]AdminUserRow, error) {
	var rows []AdminUserRow

	err := r.db.WithContext(ctx).Table("users AS u").
		Select(`u.id::text AS id, u.username, u.email, u.role, u.isactive AS is_active,
			u.avatarurl AS avatar_url, u.bio, u.authprovider AS auth_provider,
			u.createdat::text AS created_at,
			COUNT(e.courseslug) AS enrolled_courses`).
		Joins("JOIN user_groups ug ON ug.userid = u.id").
		Joins("LEFT JOIN enrollments e ON e.userid = u.id").
		Where("ug.groupid = ?::uuid", groupID).
		Group("u.id, u.username, u.email, u.role, u.isactive, u.avatarurl, u.bio, u.authprovider, u.createdat").
		Order("u.username").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}

	return rows, nil
}

// AddMember links userID to the group identified by groupID, doing nothing
// if the membership already exists.
func (r *gormGroupRepository) AddMember(ctx context.Context, groupID, userID string) error {
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return fmt.Errorf("parse group id: %w", err)
	}

	link := models.UserGroup{UserID: userID, GroupID: groupUUID}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("add group member: %w", err)
	}

	return nil
}

// RemoveMember unlinks userID from the group identified by groupID.
func (r *gormGroupRepository) RemoveMember(ctx context.Context, groupID, userID string) error {
	err := r.db.WithContext(ctx).
		Where("groupid = ?::uuid AND userid = ?::uuid", groupID, userID).
		Delete(&models.UserGroup{}).Error
	if err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}

	return nil
}

// GetGroupsByUserID returns all groups the given user belongs to.
func (r *gormGroupRepository) GetGroupsByUserID(ctx context.Context, userID string) ([]GroupRow, error) {
	var rows []GroupRow

	err := r.db.WithContext(ctx).Table("groups AS g").
		Select(`g.id::text AS id, g.name, g.source,
			COALESCE(g.parent_id::text, '') AS parent_id,
			g.depth,
			g.createdat::text AS created_at,
			COUNT(ug2.userid) AS member_count,
			COALESCE(grm.platformrole, '') AS mapped_role`).
		Joins("JOIN user_groups ug ON ug.groupid = g.id").
		Joins("LEFT JOIN user_groups ug2 ON ug2.groupid = g.id").
		Joins("LEFT JOIN group_role_mappings grm ON grm.groupname = g.name").
		Where("ug.userid = ?::uuid", userID).
		Group("g.id, g.name, g.source, g.parent_id, g.depth, g.createdat, grm.platformrole").
		Order("g.depth, g.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get groups by user id: %w", err)
	}

	return rows, nil
}

// ListMemberIDs returns the user IDs (as strings) of every member of groupID.
func (r *gormGroupRepository) ListMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	var ids []string

	err := r.db.WithContext(ctx).Table("user_groups").
		Select("userid::text").
		Where("groupid = ?::uuid", groupID).
		Pluck("userid::text", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("list member ids: %w", err)
	}

	return ids, nil
}

// UserInAnyGroup reports whether userID belongs to at least one of the
// groups listed in groupIDs.
func (r *gormGroupRepository) UserInAnyGroup(ctx context.Context, userID string, groupIDs []string) (bool, error) {
	if len(groupIDs) == 0 {
		return false, nil
	}

	var count int64

	err := r.db.WithContext(ctx).Model(&models.UserGroup{}).
		Where("userid = ?::uuid AND groupid::text = ANY(?)", userID, pq.Array(groupIDs)).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check group membership: %w", err)
	}

	return count > 0, nil
}

// HasMembers reports whether the group identified by id has any user members.
func (r *gormGroupRepository) HasMembers(ctx context.Context, id string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).Model(&models.UserGroup{}).
		Where("groupid = ?::uuid", id).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check members: %w", err)
	}

	return count > 0, nil
}

// ListMembersRecursive returns members of groupID and all its subgroups,
// flagging whether each user is a direct member of groupID.
func (r *gormGroupRepository) ListMembersRecursive(ctx context.Context, groupID string) ([]GroupMemberRow, error) {
	var group models.Group

	findErr := r.db.WithContext(ctx).Where("id = ?::uuid", groupID).First(&group).Error
	if findErr != nil {
		return nil, fmt.Errorf("get group for recursive members: %w", findErr)
	}

	var rows []GroupMemberRow

	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`DISTINCT u.id::text AS id, u.username, u.email, u.role,
			u.isactive AS is_active, u.avatarurl AS avatar_url, u.bio,
			u.authprovider AS auth_provider, u.createdat::text AS created_at,
			0 AS enrolled_courses,
			EXISTS (
				SELECT 1 FROM user_groups ug2
				WHERE ug2.userid = u.id AND ug2.groupid = ?::uuid
			) AS direct_membership`, groupID).
		Joins("JOIN user_groups ug ON ug.userid = u.id").
		Joins("JOIN groups g ON g.id = ug.groupid").
		Where("g.id = ?::uuid OR g.path LIKE ?", groupID, group.Path+"/%").
		Order("u.username").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list recursive members: %w", err)
	}

	return rows, nil
}
