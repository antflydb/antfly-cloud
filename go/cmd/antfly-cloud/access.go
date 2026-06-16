package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newAccessCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "access", Short: "Manage organization members and Antfly Cloud access"}
	cmd.AddCommand(newAccessMembersCommand(stateFor))
	cmd.AddCommand(newAccessGroupsCommand(stateFor))
	cmd.AddCommand(newAccessGrantsCommand(stateFor))
	cmd.AddCommand(newAccessAttributesCommand(stateFor))
	cmd.AddCommand(newAccessSCIMCommand(stateFor))
	return cmd
}

func newAccessMembersCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "members", Short: "Manage organization members"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List organization members", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		members, err := st.client.OrganizationMembers(cmd.Context(), org.ID)
		if err != nil {
			return err
		}
		return st.out.Print(members, func() error {
			rows := make([][]string, 0, len(members))
			for _, member := range members {
				rows = append(rows, []string{member.Email, member.DisplayName, member.Role, member.Status, shortID(member.UserID), shortID(member.ID)})
			}
			table(st.out.W, []string{"EMAIL", "NAME", "ROLE", "STATUS", "USER", "MEMBER"}, rows)
			return nil
		})
	}})
	var inviteRole string
	invite := &cobra.Command{Use: "invite <email>", Short: "Invite a user to the organization", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		inv, err := st.client.InviteOrganizationMember(cmd.Context(), org.ID, args[0], inviteRole)
		if err != nil {
			return err
		}
		return st.out.Print(inv, func() error {
			fmt.Fprintf(st.out.W, "Invited %s as %s\n", inv.Email, inv.Role)
			return nil
		})
	}}
	invite.Flags().StringVar(&inviteRole, "role", "developer", "organization role: admin or developer")
	cmd.AddCommand(invite)
	var role string
	roleCmd := &cobra.Command{Use: "role <member-id-or-user-id-or-email>", Short: "Update a member role", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !validOrgMemberRole(role) {
			return fmt.Errorf("role must be admin or developer")
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		member, err := resolveOrganizationMember(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.UpdateMemberRole(cmd.Context(), org.ID, member.ID, role)
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Updated %s role to %s\n", member.Email, updated.Role)
			return nil
		})
	}}
	roleCmd.Flags().StringVar(&role, "role", "", "organization role: admin or developer")
	_ = roleCmd.MarkFlagRequired("role")
	cmd.AddCommand(roleCmd)
	var metadataJSON string
	metadataCmd := &cobra.Command{Use: "metadata <member-id-or-user-id-or-email>", Short: "Replace organization-scoped member metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		metadata, err := parseJSONMap(metadataJSON)
		if err != nil {
			return err
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		member, err := resolveOrganizationMember(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.UpdateMemberMetadata(cmd.Context(), org.ID, member.ID, metadata)
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Updated metadata for %s\n", member.Email)
			return nil
		})
	}}
	metadataCmd.Flags().StringVar(&metadataJSON, "json", "", "metadata JSON object")
	_ = metadataCmd.MarkFlagRequired("json")
	cmd.AddCommand(metadataCmd)
	cmd.AddCommand(&cobra.Command{Use: "remove <member-id-or-user-id-or-email>", Short: "Remove a member from the organization", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		member, err := resolveOrganizationMember(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		if err := st.client.RemoveOrganizationMember(cmd.Context(), org.ID, member.ID); err != nil {
			return err
		}
		fmt.Fprintf(st.out.W, "Removed %s\n", member.Email)
		return nil
	}})
	return cmd
}

func newAccessGroupsCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "groups", Short: "Manage Antfly Cloud RBAC groups"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List Cloud RBAC groups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		groups, err := st.client.CloudGroups(cmd.Context(), org.ID)
		if err != nil {
			return err
		}
		return st.out.Print(groups, func() error {
			rows := make([][]string, 0, len(groups))
			for _, group := range groups {
				rows = append(rows, []string{group.Name, group.Slug, shortID(group.ID), displayOrDash(group.ExternalID), displayOrDash(group.Description)})
			}
			table(st.out.W, []string{"NAME", "SLUG", "ID", "EXTERNAL", "DESCRIPTION"}, rows)
			return nil
		})
	}})
	var slug, description, metadataJSON string
	create := &cobra.Command{Use: "create <name>", Short: "Create a Cloud RBAC group", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		metadata, err := optionalJSONMap(metadataJSON)
		if err != nil {
			return err
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		group, err := st.client.CreateCloudGroup(cmd.Context(), org.ID, CreateCloudGroupRequest{Name: args[0], Slug: slug, Description: description, Metadata: metadata})
		if err != nil {
			return err
		}
		return st.out.Print(group, func() error {
			fmt.Fprintf(st.out.W, "Created group %s (%s)\n", group.Name, group.ID)
			return nil
		})
	}}
	create.Flags().StringVar(&slug, "slug", "", "group slug")
	create.Flags().StringVar(&description, "description", "", "group description")
	create.Flags().StringVar(&metadataJSON, "metadata", "", "metadata JSON object")
	cmd.AddCommand(create)
	update := &cobra.Command{Use: "update <group-id-or-slug>", Short: "Update a Cloud RBAC group", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		metadata, err := optionalJSONMap(metadataJSON)
		if err != nil {
			return err
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		group, err := resolveCloudGroup(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.UpdateCloudGroup(cmd.Context(), org.ID, group.ID, UpdateCloudGroupRequest{Name: strings.TrimSpace(slug), Description: description, Metadata: metadata})
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Updated group %s\n", updated.Name)
			return nil
		})
	}}
	update.Flags().StringVar(&slug, "name", "", "new group display name")
	update.Flags().StringVar(&description, "description", "", "group description")
	update.Flags().StringVar(&metadataJSON, "metadata", "", "metadata JSON object")
	cmd.AddCommand(update)
	cmd.AddCommand(newAccessGroupMembersCommand(stateFor))
	return cmd
}

func newAccessGroupMembersCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "members", Short: "Manage Cloud RBAC group members"}
	cmd.AddCommand(&cobra.Command{Use: "list <group-id-or-slug>", Short: "List group members", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, group, err := accessGroup(cmd, stateFor, args[0])
		if err != nil {
			return err
		}
		members, err := st.client.CloudGroupMembers(cmd.Context(), org.ID, group.ID)
		if err != nil {
			return err
		}
		return st.out.Print(members, func() error {
			rows := make([][]string, 0, len(members))
			for _, member := range members {
				rows = append(rows, []string{shortID(member.UserID), fmtTime(member.AddedAt), shortID(member.AddedBy)})
			}
			table(st.out.W, []string{"USER", "ADDED", "ADDED_BY"}, rows)
			return nil
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "add <group-id-or-slug> <user-id-or-email>", Short: "Add a user to a Cloud RBAC group", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, group, err := accessGroup(cmd, stateFor, args[0])
		if err != nil {
			return err
		}
		userID, err := resolveOrganizationUserID(cmd, st, org.ID, args[1])
		if err != nil {
			return err
		}
		member, err := st.client.AddCloudGroupMember(cmd.Context(), org.ID, group.ID, userID)
		if err != nil {
			return err
		}
		return st.out.Print(member, func() error {
			fmt.Fprintf(st.out.W, "Added %s to %s\n", member.UserID, group.Slug)
			return nil
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "remove <group-id-or-slug> <user-id-or-email>", Short: "Remove a user from a Cloud RBAC group", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, group, err := accessGroup(cmd, stateFor, args[0])
		if err != nil {
			return err
		}
		userID, err := resolveOrganizationUserID(cmd, st, org.ID, args[1])
		if err != nil {
			return err
		}
		if err := st.client.RemoveCloudGroupMember(cmd.Context(), org.ID, group.ID, userID); err != nil {
			return err
		}
		fmt.Fprintf(st.out.W, "Removed %s from %s\n", userID, group.Slug)
		return nil
	}})
	return cmd
}

func newAccessGrantsCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "grants", Short: "Manage instance grants"}
	cmd.AddCommand(&cobra.Command{Use: "list [instance-id-or-slug]", Short: "List instance grants", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := accessInstance(cmd, stateFor, args)
		if err != nil {
			return err
		}
		grants, err := st.client.CloudGrants(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(grants, func() error {
			rows := make([][]string, 0, len(grants))
			for _, grant := range grants {
				rows = append(rows, []string{shortID(grant.ID), grant.SubjectType, shortID(grant.SubjectID), grant.TableName, strings.Join(grant.Actions, ",")})
			}
			table(st.out.W, []string{"ID", "SUBJECT_TYPE", "SUBJECT", "TABLE", "ACTIONS"}, rows)
			return nil
		})
	}})
	var subjectType, subjectID, tableName, rowFilterJSON, rowFilterTemplateJSON, grantMetadataJSON string
	var actions []string
	upsert := &cobra.Command{Use: "upsert [instance-id-or-slug]", Short: "Create or update an instance grant", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !validGrantSubjectType(subjectType) {
			return fmt.Errorf("subject type must be user, group, or cloud_api_key")
		}
		st, org, inst, err := accessInstance(cmd, stateFor, args)
		if err != nil {
			return err
		}
		resolvedSubjectID, err := resolveGrantSubjectID(cmd, st, org.ID, subjectType, subjectID)
		if err != nil {
			return err
		}
		rowFilter, err := optionalJSONMap(rowFilterJSON)
		if err != nil {
			return err
		}
		rowFilterTemplate, err := optionalJSONMap(rowFilterTemplateJSON)
		if err != nil {
			return err
		}
		metadata, err := optionalJSONMap(grantMetadataJSON)
		if err != nil {
			return err
		}
		grant, err := st.client.UpsertCloudGrant(cmd.Context(), org.ID, inst.ID, UpsertCloudGrantRequest{
			SubjectType: subjectType, SubjectID: resolvedSubjectID, TableName: tableName, Actions: actions, RowFilter: rowFilter, RowFilterTemplate: rowFilterTemplate, Metadata: metadata,
		})
		if err != nil {
			return err
		}
		return st.out.Print(grant, func() error {
			fmt.Fprintf(st.out.W, "Saved grant %s\n", grant.ID)
			return nil
		})
	}}
	upsert.Flags().StringVar(&subjectType, "subject-type", "", "subject type: user, group, or cloud_api_key")
	upsert.Flags().StringVar(&subjectID, "subject", "", "subject UUID, group slug, or member email for user grants")
	upsert.Flags().StringVar(&tableName, "table", "*", "table name or *")
	upsert.Flags().StringSliceVar(&actions, "action", nil, "grant action or preset; repeat or comma-separate")
	upsert.Flags().StringVar(&rowFilterJSON, "row-filter", "", "static row filter JSON object")
	upsert.Flags().StringVar(&rowFilterTemplateJSON, "row-filter-template", "", "row filter template JSON object")
	upsert.Flags().StringVar(&grantMetadataJSON, "metadata", "", "metadata JSON object")
	_ = upsert.MarkFlagRequired("subject-type")
	_ = upsert.MarkFlagRequired("subject")
	_ = upsert.MarkFlagRequired("action")
	cmd.AddCommand(upsert)
	cmd.AddCommand(&cobra.Command{Use: "delete <grant-id> [instance-id-or-slug]", Short: "Delete an instance grant", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		instanceArgs := []string{}
		if len(args) == 2 {
			instanceArgs = []string{args[1]}
		}
		st, org, inst, err := accessInstance(cmd, stateFor, instanceArgs)
		if err != nil {
			return err
		}
		if err := st.client.DeleteCloudGrant(cmd.Context(), org.ID, inst.ID, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(st.out.W, "Deleted grant %s\n", args[0])
		return nil
	}})
	return cmd
}

func newAccessAttributesCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{Use: "attributes", Short: "Manage Cloud user attributes"}
	cmd.AddCommand(&cobra.Command{Use: "get <user-id-or-email>", Short: "Show user attributes", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		userID, err := resolveOrganizationUserID(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		attrs, err := st.client.CloudUserAttributes(cmd.Context(), org.ID, userID)
		if err != nil {
			return err
		}
		return st.out.Print(attrs, func() error { return st.out.PrintJSON(attrs) })
	}})
	var attrsJSON string
	set := &cobra.Command{Use: "set <user-id-or-email>", Short: "Replace manual user attributes", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manual, err := parseJSONMap(attrsJSON)
		if err != nil {
			return err
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		userID, err := resolveOrganizationUserID(cmd, st, org.ID, args[0])
		if err != nil {
			return err
		}
		attrs, err := st.client.UpdateCloudUserAttributes(cmd.Context(), org.ID, userID, manual)
		if err != nil {
			return err
		}
		return st.out.Print(attrs, func() error {
			fmt.Fprintf(st.out.W, "Updated attributes for %s\n", userID)
			return nil
		})
	}}
	set.Flags().StringVar(&attrsJSON, "json", "", "manual attributes JSON object")
	_ = set.MarkFlagRequired("json")
	cmd.AddCommand(set)
	return cmd
}

func newAccessSCIMCommand(stateFor stateFactory) *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "scim-sync", Short: "Sync SCIM-managed groups from JSON", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req CloudSCIMGroupSyncRequest
		if err := readJSONFile(file, &req); err != nil {
			return err
		}
		st, org, err := accessOrg(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.SyncCloudSCIMGroups(cmd.Context(), org.ID, req)
		if err != nil {
			return err
		}
		return st.out.Print(result, func() error {
			fmt.Fprintf(st.out.W, "Synced groups=%d memberships=%d attributes=%d\n", result.GroupsSynced, result.MembershipsSynced, result.AttributesSynced)
			return nil
		})
	}}
	cmd.Flags().StringVar(&file, "file", "-", "JSON file containing {\"groups\":[...]}; use - for stdin")
	return cmd
}

func accessOrg(cmd *cobra.Command, stateFor stateFactory) (*appState, Organization, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, Organization{}, err
	}
	org, err := resolveOrg(cmd.Context(), st)
	if err != nil {
		return nil, Organization{}, err
	}
	return st, org, nil
}

func accessGroup(cmd *cobra.Command, stateFor stateFactory, ref string) (*appState, Organization, CloudGroup, error) {
	st, org, err := accessOrg(cmd, stateFor)
	if err != nil {
		return nil, Organization{}, CloudGroup{}, err
	}
	group, err := resolveCloudGroup(cmd, st, org.ID, ref)
	if err != nil {
		return nil, Organization{}, CloudGroup{}, err
	}
	return st, org, group, nil
}

func accessInstance(cmd *cobra.Command, stateFor stateFactory, args []string) (*appState, Organization, CloudInstance, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	org, err := resolveOrg(cmd.Context(), st)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	ref := st.cfg.Instance
	if len(args) > 0 {
		ref = args[0]
	}
	inst, err := resolveActiveInstance(cmd.Context(), st, org.ID, ref)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	return st, org, inst, nil
}

func resolveOrganizationMember(cmd *cobra.Command, st *appState, orgID, ref string) (OrganizationMember, error) {
	members, err := st.client.OrganizationMembers(cmd.Context(), orgID)
	if err != nil {
		return OrganizationMember{}, err
	}
	for _, member := range members {
		if member.ID == ref || member.UserID == ref || strings.EqualFold(member.Email, ref) {
			return member, nil
		}
	}
	return OrganizationMember{}, fmt.Errorf("organization member %q not found", ref)
}

func resolveOrganizationUserID(cmd *cobra.Command, st *appState, orgID, ref string) (string, error) {
	if isUUID(ref) {
		return ref, nil
	}
	member, err := resolveOrganizationMember(cmd, st, orgID, ref)
	if err != nil {
		return "", err
	}
	return member.UserID, nil
}

func resolveCloudGroup(cmd *cobra.Command, st *appState, orgID, ref string) (CloudGroup, error) {
	groups, err := st.client.CloudGroups(cmd.Context(), orgID)
	if err != nil {
		return CloudGroup{}, err
	}
	for _, group := range groups {
		if group.ID == ref || group.Slug == ref || strings.EqualFold(group.Name, ref) {
			return group, nil
		}
	}
	return CloudGroup{}, fmt.Errorf("cloud group %q not found", ref)
}

func resolveGrantSubjectID(cmd *cobra.Command, st *appState, orgID, subjectType, ref string) (string, error) {
	if isUUID(ref) {
		return ref, nil
	}
	switch subjectType {
	case "user":
		return resolveOrganizationUserID(cmd, st, orgID, ref)
	case "group":
		group, err := resolveCloudGroup(cmd, st, orgID, ref)
		if err != nil {
			return "", err
		}
		return group.ID, nil
	default:
		return "", fmt.Errorf("subject %q must be a UUID for %s grants", ref, subjectType)
	}
}

func isUUID(ref string) bool {
	_, err := uuid.Parse(ref)
	return err == nil
}

func validOrgMemberRole(role string) bool {
	return role == "admin" || role == "developer"
}

func validGrantSubjectType(subjectType string) bool {
	return subjectType == "user" || subjectType == "group" || subjectType == "cloud_api_key"
}

func parseJSONMap(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("JSON object is required")
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("invalid JSON object: %w", err)
	}
	if out == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return out, nil
}

func optionalJSONMap(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return parseJSONMap(raw)
}

func readJSONFile(path string, out any) error {
	var data []byte
	var err error
	if path == "" || path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
