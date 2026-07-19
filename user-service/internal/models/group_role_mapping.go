package models

// GroupRoleMapping maps an IdP/local group name to the platform role its
// members should be granted ("admin" or "student").
type GroupRoleMapping struct {
	GroupName    string `gorm:"column:groupname;primaryKey"`
	PlatformRole string `gorm:"column:platformrole"`
}

// TableName pins GroupRoleMapping to the group_role_mappings table.
func (GroupRoleMapping) TableName() string { return "group_role_mappings" }
