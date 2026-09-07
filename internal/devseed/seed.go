// Package devseed creates repeatable, clearly-prefixed local data for the
// development environment. It never deletes rows and never treats demo data
// as a migration or production provisioning mechanism.
package devseed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	DefaultSeed           int64 = 20260903
	DefaultPersonnelCount       = 96
	DefaultFlightCount          = 72
	DefaultPrefix               = "DEMO"
	DefaultPassword             = "Flight123!"
)

var ErrNoSeedData = errors.New("no seeded Core data found")

type Options struct {
	Seed           int64
	PersonnelCount int
	FlightCount    int
	Prefix         string
	Password       string
	Now            time.Time
}

type AreaSeed struct {
	ID       uint64
	PublicID string
	Code     string
	Name     string
}

type TeamSeed struct {
	ID           uint64
	PublicID     string
	AreaID       uint64
	AreaPublicID string
	Code         string
	Name         string
	Position     string
	Capabilities []string
}

type PersonnelSeed struct {
	ID           uint64
	PublicID     string
	UserPublicID string
	EmployeeNo   string
	DisplayName  string
	TeamID       uint64
	TeamPublicID string
	Position     string
	Capabilities []string
}

type ProjectionSeed struct {
	PublicID           string
	AssignmentPublicID string
	EmployeePublicID   string
	FlightDisplayNo    string
	TaskName           string
	AreaName           string
	PlannedAt          time.Time
	Status             string
	Message            string
	SyncVersion        uint64
}

type Manifest struct {
	Seed        int64
	Prefix      string
	Password    string
	Areas       []AreaSeed
	Teams       []TeamSeed
	Personnel   []PersonnelSeed
	Projections []ProjectionSeed
}

type areaSpec struct {
	Code string
	Name string
}

type teamSpec struct {
	AreaIndex    int
	Code         string
	Name         string
	Position     string
	Capabilities []string
}

var areaSpecs = []areaSpec{
	{Code: "T1-DOMESTIC", Name: "T1 国内航站楼"},
	{Code: "T2-INTERNATIONAL", Name: "T2 国际航站楼"},
	{Code: "TRANSFER-CENTER", Name: "中转服务区"},
	{Code: "RAMP-SUPPORT", Name: "航班地面保障区"},
	{Code: "SALES-CONSULTING", Name: "客运销售与咨询区"},
}

var teamSpecs = []teamSpec{
	{AreaIndex: 0, Code: "CHECKIN", Name: "值机服务组", Position: "checkin_agent", Capabilities: []string{"domestic_checkin", "self_service_checkin"}},
	{AreaIndex: 1, Code: "INTL-CHECKIN", Name: "国际值机服务组", Position: "checkin_agent", Capabilities: []string{"international_checkin", "document_check"}},
	{AreaIndex: 0, Code: "ARRIVAL-DEPARTURE", Name: "进出港服务组", Position: "ground_service_agent", Capabilities: []string{"arrival_service", "departure_service"}},
	{AreaIndex: 2, Code: "TRANSFER", Name: "中转服务组", Position: "transfer_agent", Capabilities: []string{"transfer_service", "connection_assistance"}},
	{AreaIndex: 2, Code: "SPECIAL-PASSENGER", Name: "特殊旅客服务组", Position: "special_service_agent", Capabilities: []string{"wheelchair_service", "unaccompanied_minor"}},
	{AreaIndex: 1, Code: "IRREGULAR-OPS", Name: "不正常航班服务组", Position: "irregular_ops_agent", Capabilities: []string{"irregular_flight", "passenger_rebooking"}},
	{AreaIndex: 3, Code: "LOAD-CONTROL", Name: "配载平衡组", Position: "load_controller", Capabilities: []string{"load_balance", "weight_balance"}},
	{AreaIndex: 3, Code: "BAGGAGE-GROUND", Name: "行李地面服务组", Position: "baggage_agent", Capabilities: []string{"baggage_delivery", "baggage_trace"}},
	{AreaIndex: 3, Code: "FLIGHT-GROUND-SUPPORT", Name: "航班地面保障组", Position: "ramp_agent", Capabilities: []string{"ground_support", "stand_coordination"}},
	{AreaIndex: 4, Code: "PASSENGER-SALES", Name: "客运销售代理组", Position: "ticketing_agent", Capabilities: []string{"passenger_sales_agency", "domestic_international_ticketing"}},
	{AreaIndex: 4, Code: "AVIATION-CONSULTING", Name: "航空信息咨询组", Position: "aviation_consultant", Capabilities: []string{"aviation_information_consulting", "flight_information_service"}},
}

var templateMessages = []string{
	"完成国内航班值机柜台准备，核对航班和值机设备状态。",
	"完成国际航班值机准备，复核证件查验和特殊文件提示。",
	"完成航班进出港服务准备，确认旅客引导和登机口信息。",
	"核对中转旅客清单，完成中转引导和衔接保障。",
	"确认特殊旅客服务需求，安排陪同、轮椅或无陪儿童服务。",
	"处理不正常航班旅客服务，完成改签、通知和现场引导。",
	"完成配载平衡数据复核，确认航班载重和舱位信息。",
	"完成到达行李地面派送，记录异常行李并反馈处理进度。",
	"完成航班地面保障协调，确认机位、登机口和保障节点。",
	"提供国内外航空客运销售代理服务，核对航班、票务和旅客信息。",
	"响应航空信息咨询，提供航班、值机、进出港和中转指引。",
}

func DefaultOptions() Options {
	return Options{Seed: DefaultSeed, PersonnelCount: DefaultPersonnelCount, FlightCount: DefaultFlightCount, Prefix: DefaultPrefix, Password: DefaultPassword, Now: time.Now().UTC()}
}

func (o Options) normalized() (Options, error) {
	if o.Seed == 0 {
		o.Seed = DefaultSeed
	}
	if o.PersonnelCount <= 0 {
		o.PersonnelCount = DefaultPersonnelCount
	}
	if o.FlightCount <= 0 {
		o.FlightCount = DefaultFlightCount
	}
	if o.PersonnelCount < len(teamSpecs)*2 {
		return Options{}, fmt.Errorf("personnel count must be at least %d", len(teamSpecs)*2)
	}
	o.Prefix = strings.TrimSpace(o.Prefix)
	if o.Prefix == "" {
		o.Prefix = DefaultPrefix
	}
	o.Password = strings.TrimSpace(o.Password)
	if len(o.Password) < 8 {
		return Options{}, errors.New("seed password must be at least 8 characters")
	}
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
	o.Now = o.Now.UTC()
	return o, nil
}

// SeedCore writes organization, employee, credential, template, flight and
// task facts in one transaction. Existing rows are updated only when their
// natural key uses the requested demo prefix.
func SeedCore(ctx context.Context, db *gorm.DB, options Options) (Manifest, error) {
	if db == nil {
		return Manifest{}, errors.New("core database is nil")
	}
	options, err := options.normalized()
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Seed: options.Seed, Prefix: options.Prefix, Password: options.Password}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(options.Password), bcrypt.MinCost)
	if err != nil {
		return Manifest{}, fmt.Errorf("hash seed password: %w", err)
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		areas := make([]AreaSeed, 0, len(areaSpecs))
		for i, spec := range areaSpecs {
			area, err := ensureArea(tx, options, i, spec)
			if err != nil {
				return err
			}
			areas = append(areas, area)
		}
		teams := make([]TeamSeed, 0, len(teamSpecs))
		for i, spec := range teamSpecs {
			team, err := ensureTeam(tx, options, i, areas[spec.AreaIndex], spec)
			if err != nil {
				return err
			}
			teams = append(teams, team)
		}
		if err := ensureReferenceDictionaries(tx, options); err != nil {
			return err
		}

		personnel := make([]PersonnelSeed, 0, options.PersonnelCount)
		for i := 0; i < options.PersonnelCount; i++ {
			team := teams[i%len(teams)]
			person, err := ensurePersonnel(tx, options, i, team, passwordHash)
			if err != nil {
				return err
			}
			personnel = append(personnel, person)
			if err := ensureTeamMember(tx, options, i, team, person); err != nil {
				return err
			}
		}
		for i := 0; i < minInt(24, len(personnel)); i++ {
			if err := ensureBinding(tx, options, personnel[i], "personal_wechat", fmt.Sprintf("%s-wx-%04d", strings.ToLower(options.Prefix), i+1)); err != nil {
				return err
			}
			if err := ensureBinding(tx, options, personnel[i], "wecom", fmt.Sprintf("%s-wecom-%04d", strings.ToLower(options.Prefix), i+1)); err != nil {
				return err
			}
		}
		if err := ensureAdminIdentities(tx, options, areas, teams, personnel); err != nil {
			return err
		}

		templates := make([]templateSeed, 0, len(teamSpecs))
		for i, spec := range teamSpecs {
			template, err := ensureTemplate(tx, options, i, areas[spec.AreaIndex], teams[i], spec)
			if err != nil {
				return err
			}
			templates = append(templates, template)
		}

		random := rand.New(rand.NewSource(options.Seed))
		projections := make([]ProjectionSeed, 0, options.FlightCount)
		personnelState := make(map[uint64]string, len(personnel))
		for _, person := range personnel {
			personnelState[person.ID] = "idle"
		}
		for i := 0; i < options.FlightCount; i++ {
			flight, err := ensureFlight(tx, options, i, random)
			if err != nil {
				return err
			}
			template := templates[i%len(templates)]
			teamPeople := peopleForTeam(personnel, template.TeamID)
			if len(teamPeople) < 3 {
				return fmt.Errorf("team %s has fewer than three seeded personnel", template.TeamPublicID)
			}
			status := seededTaskStatus(i)
			selected, err := selectSeedPersonnel(teamPeople, status, i, personnelState)
			if err != nil {
				return err
			}
			candidates := seedCandidatePool(teamPeople, selected, i)
			taskID, assignmentID, err := ensureTaskGraph(tx, options, i, flight, template, candidates, selected, status)
			if err != nil {
				return err
			}
			if assignmentID != "" {
				personnelState[selected.ID] = mergePersonnelSeedState(personnelState[selected.ID], personnelStateForTask(status))
				projection := ProjectionSeed{PublicID: taskID, AssignmentPublicID: assignmentID, EmployeePublicID: selected.PublicID, FlightDisplayNo: flight.DisplayNo, TaskName: template.Name, AreaName: template.AreaName, PlannedAt: flight.ScheduledAt.Add(time.Duration(template.OffsetSeconds) * time.Second), Status: status, Message: template.Message, SyncVersion: uint64(i%5 + 1)}
				projections = append(projections, projection)
				if err := ensureSeedOutbox(tx, options, projection); err != nil {
					return err
				}
			}
		}
		for personnelID, state := range personnelState {
			if err := tx.Table("personnel").Where("id = ?", personnelID).Updates(map[string]any{"work_state": state, "last_state_changed_at": options.Now}).Error; err != nil {
				return fmt.Errorf("update seeded personnel state: %w", err)
			}
		}
		manifest.Areas, manifest.Teams, manifest.Personnel, manifest.Projections = areas, teams, personnel, projections
		return nil
	}); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// SeedEdge creates the minimal employee projection and notification facts for
// the same manifest. Core remains the only source of business truth.
func SeedEdge(ctx context.Context, db *gorm.DB, manifest Manifest) error {
	if db == nil {
		return errors.New("edge database is nil")
	}
	if len(manifest.Projections) == 0 {
		return ErrNoSeedData
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revisions := make(map[string]uint64)
		for _, projection := range manifest.Projections {
			values := map[string]any{
				"public_id": projection.PublicID, "assignment_public_id": projection.AssignmentPublicID,
				"employee_public_id": projection.EmployeePublicID, "flight_display_no": projection.FlightDisplayNo,
				"task_name": projection.TaskName, "area_name": projection.AreaName, "planned_at": projection.PlannedAt.UTC(),
				"status": projection.Status, "message": projection.Message, "sync_version": projection.SyncVersion, "updated_at": time.Now().UTC(),
			}
			var id uint64
			if err := tx.Table("task_projection").Where("public_id = ?", projection.PublicID).Select("id").Scan(&id).Error; err != nil {
				return fmt.Errorf("find seeded Edge task projection: %w", err)
			}
			var err error
			if id == 0 {
				err = tx.Table("task_projection").Create(values).Error
			} else {
				err = tx.Table("task_projection").Where("id = ?", id).Updates(values).Error
			}
			if err != nil {
				return fmt.Errorf("write seeded Edge task projection: %w", err)
			}
			if projection.SyncVersion > revisions[projection.EmployeePublicID] {
				revisions[projection.EmployeePublicID] = projection.SyncVersion
			}
		}
		for employeePublicID, revision := range revisions {
			values := map[string]any{"employee_public_id": employeePublicID, "revision": revision, "updated_at": time.Now().UTC()}
			var cursorEmployeeID string
			if err := tx.Table("employee_projection_cursor").Where("employee_public_id = ?", employeePublicID).Select("employee_public_id").Scan(&cursorEmployeeID).Error; err != nil {
				return fmt.Errorf("find seeded Edge cursor: %w", err)
			}
			if cursorEmployeeID == "" {
				if err := tx.Table("employee_projection_cursor").Create(values).Error; err != nil {
					return fmt.Errorf("create seeded Edge cursor: %w", err)
				}
			} else if err := tx.Table("employee_projection_cursor").Where("employee_public_id = ? AND revision < ?", employeePublicID, revision).Updates(values).Error; err != nil {
				return fmt.Errorf("update seeded Edge cursor: %w", err)
			}
		}
		for i, projection := range manifest.Projections {
			if i%3 != 0 {
				continue
			}
			values := map[string]any{
				"public_id":          deterministicID(manifest.Prefix, manifest.Seed, "notification", i),
				"employee_public_id": projection.EmployeePublicID,
				"title":              "任务状态更新",
				"message":            projection.Message,
				"status":             "unread",
				"sync_version":       projection.SyncVersion,
				"updated_at":         time.Now().UTC(),
			}
			var id uint64
			if err := tx.Table("notification_projection").Where("public_id = ?", values["public_id"]).Select("id").Scan(&id).Error; err != nil {
				return fmt.Errorf("find seeded notification: %w", err)
			}
			var err error
			if id == 0 {
				err = tx.Table("notification_projection").Create(values).Error
			} else {
				err = tx.Table("notification_projection").Where("id = ?", id).Updates(values).Error
			}
			if err != nil {
				return fmt.Errorf("write seeded notification: %w", err)
			}
		}
		return nil
	})
}

type templateSeed struct {
	ID            uint64
	PublicID      string
	Name          string
	AreaID        uint64
	AreaPublicID  string
	AreaName      string
	TeamID        uint64
	TeamPublicID  string
	Position      string
	Capabilities  []string
	OffsetSeconds int
	Message       string
}

func ensureArea(tx *gorm.DB, options Options, index int, spec areaSpec) (AreaSeed, error) {
	code := seedCode(options.Prefix, spec.Code)
	var row struct {
		ID                   uint64
		PublicID, Code, Name string
	}
	err := tx.Table("operation_area").Where("code = ?", code).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.PublicID = deterministicID(options.Prefix, options.Seed, "area", index)
		if err := tx.Table("operation_area").Create(map[string]any{"public_id": row.PublicID, "code": code, "name": spec.Name, "enabled": true}).Error; err != nil {
			return AreaSeed{}, fmt.Errorf("create seed area %s: %w", code, err)
		}
		if err := tx.Table("operation_area").Where("code = ?", code).First(&row).Error; err != nil {
			return AreaSeed{}, fmt.Errorf("reload seed area %s: %w", code, err)
		}
	} else if err != nil {
		return AreaSeed{}, fmt.Errorf("find seed area %s: %w", code, err)
	} else if row.PublicID == deterministicID(options.Prefix, options.Seed, "area", index) {
		if err := tx.Table("operation_area").Where("id = ?", row.ID).Updates(map[string]any{"name": spec.Name, "enabled": true}).Error; err != nil {
			return AreaSeed{}, fmt.Errorf("update seed area %s: %w", code, err)
		}
		row.Name = spec.Name
	}
	return AreaSeed{ID: row.ID, PublicID: row.PublicID, Code: code, Name: row.Name}, nil
}

func ensureTeam(tx *gorm.DB, options Options, index int, area AreaSeed, spec teamSpec) (TeamSeed, error) {
	code := seedCode(options.Prefix, spec.Code)
	var row struct {
		ID, AreaID           uint64
		PublicID, Code, Name string
	}
	err := tx.Table("team").Where("area_id = ? AND code = ?", area.ID, code).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.PublicID = deterministicID(options.Prefix, options.Seed, "team", index)
		if err := tx.Table("team").Create(map[string]any{"public_id": row.PublicID, "area_id": area.ID, "code": code, "name": spec.Name, "enabled": true}).Error; err != nil {
			return TeamSeed{}, fmt.Errorf("create seed team %s: %w", code, err)
		}
		if err := tx.Table("team").Where("area_id = ? AND code = ?", area.ID, code).First(&row).Error; err != nil {
			return TeamSeed{}, fmt.Errorf("reload seed team %s: %w", code, err)
		}
	} else if err != nil {
		return TeamSeed{}, fmt.Errorf("find seed team %s: %w", code, err)
	} else if row.PublicID == deterministicID(options.Prefix, options.Seed, "team", index) {
		if err := tx.Table("team").Where("id = ?", row.ID).Updates(map[string]any{"name": spec.Name, "enabled": true}).Error; err != nil {
			return TeamSeed{}, fmt.Errorf("update seed team %s: %w", code, err)
		}
		row.Name = spec.Name
	}
	return TeamSeed{ID: row.ID, PublicID: row.PublicID, AreaID: area.ID, AreaPublicID: area.PublicID, Code: code, Name: row.Name, Position: spec.Position, Capabilities: append([]string(nil), spec.Capabilities...)}, nil
}

func ensurePersonnel(tx *gorm.DB, options Options, index int, team TeamSeed, passwordHash []byte) (PersonnelSeed, error) {
	// Employee numbers model the enterprise identifier and therefore stay
	// numeric even though other demo natural keys retain the readable prefix.
	digest := sha256.Sum256([]byte(options.Prefix))
	employeeNo := fmt.Sprintf("%08d", 10000000+(int(digest[0])*256+int(digest[1]))%70000000+index)
	displayName := seededName(index)
	userPublicID := deterministicID(options.Prefix, options.Seed, "user", index)
	personnelPublicID := deterministicID(options.Prefix, options.Seed, "personnel", index)
	capabilities, err := json.Marshal(primaryCapabilities(team.Capabilities))
	if err != nil {
		return PersonnelSeed{}, fmt.Errorf("marshal seed capabilities: %w", err)
	}
	var row struct {
		ID                                              uint64
		PublicID, UserPublicID, EmployeeNo, DisplayName string
	}
	err = tx.Table("personnel").Where("employee_no = ?", employeeNo).First(&row).Error
	values := map[string]any{"public_id": personnelPublicID, "user_public_id": userPublicID, "employee_no": employeeNo, "display_name": displayName, "team_id": team.ID, "position_code": team.Position, "capabilities": capabilities, "work_state": "idle", "status_version": 0, "last_state_changed_at": options.Now, "unavailable_reason": nil, "enabled": true}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Table("personnel").Create(values).Error; err != nil {
			return PersonnelSeed{}, fmt.Errorf("create seed personnel %s: %w", employeeNo, err)
		}
		if err := tx.Table("personnel").Where("employee_no = ?", employeeNo).First(&row).Error; err != nil {
			return PersonnelSeed{}, fmt.Errorf("reload seed personnel %s: %w", employeeNo, err)
		}
	} else if err != nil {
		return PersonnelSeed{}, fmt.Errorf("find seed personnel %s: %w", employeeNo, err)
	} else if row.PublicID == personnelPublicID {
		if err := tx.Table("personnel").Where("id = ?", row.ID).Updates(values).Error; err != nil {
			return PersonnelSeed{}, fmt.Errorf("update seed personnel %s: %w", employeeNo, err)
		}
	}
	var credentialID uint64
	if err := tx.Table("employee_credential").Where("personnel_id = ?", row.ID).Select("id").Scan(&credentialID).Error; err != nil {
		return PersonnelSeed{}, fmt.Errorf("find seed credential %s: %w", employeeNo, err)
	}
	credentialValues := map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "credential", index), "personnel_id": row.ID, "password_hash": string(passwordHash), "failed_attempts": 0, "locked_until": nil, "password_changed_at": options.Now, "updated_at": options.Now}
	if credentialID == 0 {
		credentialValues["created_at"] = options.Now
		if err := tx.Table("employee_credential").Create(credentialValues).Error; err != nil {
			return PersonnelSeed{}, fmt.Errorf("create seed credential %s: %w", employeeNo, err)
		}
	} else if err := tx.Table("employee_credential").Where("id = ?", credentialID).Updates(credentialValues).Error; err != nil {
		return PersonnelSeed{}, fmt.Errorf("update seed credential %s: %w", employeeNo, err)
	}
	return PersonnelSeed{ID: row.ID, PublicID: row.PublicID, UserPublicID: row.UserPublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, TeamID: team.ID, TeamPublicID: team.PublicID, Position: team.Position, Capabilities: primaryCapabilities(team.Capabilities)}, nil
}

func ensureTeamMember(tx *gorm.DB, options Options, index int, team TeamSeed, personnel PersonnelSeed) error {
	// Keep the membership natural key stable across reruns on different days;
	// demo data should be additive/idempotent, not create a new membership daily.
	joinedAt := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC).AddDate(0, 0, int(options.Seed%365)).AddDate(0, 0, -30)
	publicID := deterministicID(options.Prefix, options.Seed, "team-member", index)
	var seededRow struct {
		ID       uint64
		JoinedAt time.Time
	}
	if err := tx.Table("team_member").Where("public_id = ?", publicID).First(&seededRow).Error; err == nil {
		if seededRow.JoinedAt.Equal(joinedAt) {
			return nil
		}
		if err := tx.Table("team_member").Where("id = ?", seededRow.ID).Updates(map[string]any{"joined_at": joinedAt}).Error; err != nil {
			return fmt.Errorf("update seed team member %s: %w", personnel.EmployeeNo, err)
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find seed team member by public id %s: %w", personnel.EmployeeNo, err)
	}
	var id uint64
	if err := tx.Table("team_member").Where("team_id = ? AND personnel_id = ? AND joined_at = ?", team.ID, personnel.ID, joinedAt).Select("id").Scan(&id).Error; err != nil {
		return fmt.Errorf("find seed team member %s: %w", personnel.EmployeeNo, err)
	}
	if id != 0 {
		return nil
	}
	if err := tx.Table("team_member").Create(map[string]any{"public_id": publicID, "team_id": team.ID, "personnel_id": personnel.ID, "is_primary": true, "joined_at": joinedAt}).Error; err != nil {
		return fmt.Errorf("create seed team member %s: %w", personnel.EmployeeNo, err)
	}
	return nil
}

func ensureBinding(tx *gorm.DB, options Options, personnel PersonnelSeed, provider, subject string) error {
	var row struct {
		ID          uint64
		PersonnelID uint64
	}
	err := tx.Table("external_identity_binding").Where("provider = ? AND provider_app = ? AND external_subject = ?", provider, "local", subject).First(&row).Error
	if err == nil {
		if row.PersonnelID != personnel.ID {
			return fmt.Errorf("seed identity %s is already bound to another personnel", subject)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find seed identity %s: %w", subject, err)
	}
	var providerRow struct {
		ID              uint64
		ExternalSubject string
	}
	err = tx.Table("external_identity_binding").Where("personnel_id = ? AND provider = ? AND provider_app = ?", personnel.ID, provider, "local").First(&providerRow).Error
	if err == nil {
		if providerRow.ExternalSubject != subject {
			legacySubject := "mock:" + subject
			if providerRow.ExternalSubject != legacySubject {
				return fmt.Errorf("seed personnel %s already has a different %s identity", personnel.EmployeeNo, provider)
			}
			if err := tx.Table("external_identity_binding").Where("id = ?", providerRow.ID).Update("external_subject", subject).Error; err != nil {
				return fmt.Errorf("update legacy seed identity %s: %w", subject, err)
			}
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find seed personnel identity %s: %w", personnel.EmployeeNo, err)
	}
	if err := tx.Table("external_identity_binding").Create(map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "binding-"+provider, int(personnel.ID)), "personnel_id": personnel.ID, "provider": provider, "provider_app": "local", "external_subject": subject}).Error; err != nil {
		return fmt.Errorf("create seed identity %s: %w", subject, err)
	}
	return nil
}

func ensureAdminIdentities(tx *gorm.DB, options Options, areas []AreaSeed, teams []TeamSeed, personnel []PersonnelSeed) error {
	if len(personnel) < 4 {
		return errors.New("not enough personnel for seeded admin identities")
	}
	adminEntries := []struct {
		Index   int
		Subject string
		Name    string
		Role    string
		Global  bool
		AreaIDs []uint64
		TeamIDs []uint64
	}{
		{Index: 0, Subject: "demo-admin", Name: "演示系统管理员", Role: "admin", Global: true},
		{Index: 1, Subject: "demo-manager", Name: "演示主任", Role: "manager", Global: true},
		{Index: 2, Subject: "demo-leader-checkin", Name: "值机服务队长", Role: "leader", TeamIDs: []uint64{teams[0].ID}},
		{Index: 3, Subject: "demo-leader-ground", Name: "地面保障队长", Role: "leader", AreaIDs: []uint64{areas[3].ID}},
	}
	for _, entry := range adminEntries {
		areaIDs, err := json.Marshal(entry.AreaIDs)
		if err != nil {
			return fmt.Errorf("marshal admin area scope: %w", err)
		}
		teamIDs, err := json.Marshal(entry.TeamIDs)
		if err != nil {
			return fmt.Errorf("marshal admin team scope: %w", err)
		}
		values := map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "admin-identity", entry.Index), "provider": "development", "external_subject": entry.Subject, "display_name": entry.Name, "role": entry.Role, "enabled": true, "global_scope": entry.Global, "area_ids": areaIDs, "team_ids": teamIDs, "user_id": personnel[entry.Index].ID}
		var id uint64
		if err := tx.Table("admin_identity").Where("provider = ? AND external_subject = ?", "development", entry.Subject).Select("id").Scan(&id).Error; err != nil {
			return fmt.Errorf("find seed admin identity %s: %w", entry.Subject, err)
		}
		if id == 0 {
			if err := tx.Table("admin_identity").Create(values).Error; err != nil {
				return fmt.Errorf("create seed admin identity %s: %w", entry.Subject, err)
			}
		} else if err := tx.Table("admin_identity").Where("id = ?", id).Updates(values).Error; err != nil {
			return fmt.Errorf("update seed admin identity %s: %w", entry.Subject, err)
		}
	}
	return nil
}

func ensureTemplate(tx *gorm.DB, options Options, index int, area AreaSeed, team TeamSeed, spec teamSpec) (templateSeed, error) {
	version := 100 + index
	name := templateMessages[index]
	capabilities, err := json.Marshal(primaryCapabilities(spec.Capabilities))
	if err != nil {
		return templateSeed{}, err
	}
	values := map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "template", index), "name": name, "trigger_type": "flight_arrived", "template_version": version, "enabled": true, "target_area_id": area.ID, "target_team_id": team.ID, "required_position_code": spec.Position, "required_capabilities": capabilities, "planned_offset_seconds": 900 + index*180, "default_message": name, "created_by_public_id": deterministicID(options.Prefix, options.Seed, "admin-identity", 1)}
	var row struct {
		ID       uint64
		PublicID string
	}
	err = tx.Table("task_template").Where("trigger_type = ? AND template_version = ?", "flight_arrived", version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Table("task_template").Create(values).Error; err != nil {
			return templateSeed{}, fmt.Errorf("create seed template %s: %w", name, err)
		}
		if err := tx.Table("task_template").Where("trigger_type = ? AND template_version = ?", "flight_arrived", version).First(&row).Error; err != nil {
			return templateSeed{}, fmt.Errorf("reload seed template %s: %w", name, err)
		}
	} else if err != nil {
		return templateSeed{}, fmt.Errorf("find seed template %s: %w", name, err)
	} else if row.PublicID == deterministicID(options.Prefix, options.Seed, "template", index) {
		if err := tx.Table("task_template").Where("id = ?", row.ID).Updates(values).Error; err != nil {
			return templateSeed{}, fmt.Errorf("update seed template %s: %w", name, err)
		}
	}
	return templateSeed{ID: row.ID, PublicID: row.PublicID, Name: name, AreaID: area.ID, AreaPublicID: area.PublicID, AreaName: area.Name, TeamID: team.ID, TeamPublicID: team.PublicID, Position: spec.Position, Capabilities: primaryCapabilities(spec.Capabilities), OffsetSeconds: 900 + index*180, Message: name}, nil
}

func primaryCapabilities(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return []string{values[0]}
}

func ensureReferenceDictionaries(tx *gorm.DB, options Options) error {
	positionSeen := make(map[string]struct{}, len(teamSpecs))
	capabilitySeen := make(map[string]struct{}, len(teamSpecs)*2)
	for index, spec := range teamSpecs {
		if _, ok := positionSeen[spec.Position]; !ok {
			if err := ensureReferenceDictionary(tx, "job_position", deterministicID(options.Prefix, options.Seed, "position", index), spec.Position, spec.Position); err != nil {
				return err
			}
			positionSeen[spec.Position] = struct{}{}
		}
		for offset, capability := range spec.Capabilities {
			if _, ok := capabilitySeen[capability]; ok {
				continue
			}
			if err := ensureReferenceDictionary(tx, "capability", deterministicID(options.Prefix, options.Seed, "capability", index*10+offset), capability, capability); err != nil {
				return err
			}
			capabilitySeen[capability] = struct{}{}
		}
	}
	return nil
}

func ensureReferenceDictionary(tx *gorm.DB, table, publicID, code, name string) error {
	var row struct {
		ID       uint64
		PublicID string
	}
	if err := tx.Table(table).Where("code = ?", code).First(&row).Error; err == nil {
		if row.PublicID == publicID {
			return tx.Table(table).Where("id = ?", row.ID).Updates(map[string]any{"name": name, "enabled": true}).Error
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find seed %s %s: %w", table, code, err)
	}
	if err := tx.Table(table).Create(map[string]any{"public_id": publicID, "code": code, "name": name, "description": "", "enabled": true}).Error; err != nil {
		return fmt.Errorf("create seed %s %s: %w", table, code, err)
	}
	return nil
}

func ensureFlight(tx *gorm.DB, options Options, index int, random *rand.Rand) (flightSeed, error) {
	airlines := []string{"CA", "MU", "CZ", "HU", "ZH", "3U", "MF", "9C"}
	flightDate := time.Date(options.Now.Year(), options.Now.Month(), options.Now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, (index%7)-2)
	flightNo := fmt.Sprintf("%s%s%04d", options.Prefix, airlines[random.Intn(len(airlines))], 1000+index)
	scheduledAt := flightDate.Add(time.Duration(6+random.Intn(16))*time.Hour + time.Duration(random.Intn(12))*5*time.Minute)
	status := []string{"scheduled", "scheduled", "arrived", "arrived", "departed", "cancelled"}[index%6]
	values := map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "flight", index), "flight_display_no": flightNo, "source_provider": "development", "external_flight_id": deterministicID(options.Prefix, options.Seed, "external-flight", index), "operating_date": flightDate, "scheduled_at": scheduledAt, "status": status, "status_version": 0, "last_status_changed_at": options.Now}
	var row struct {
		ID                          uint64
		PublicID, DisplayNo, Status string
		ScheduledAt                 time.Time
	}
	err := tx.Table("flight").Where("operating_date = ? AND flight_display_no = ?", flightDate, flightNo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Table("flight").Create(values).Error; err != nil {
			return flightSeed{}, fmt.Errorf("create seed flight %s: %w", flightNo, err)
		}
		if err := tx.Table("flight").Where("operating_date = ? AND flight_display_no = ?", flightDate, flightNo).First(&row).Error; err != nil {
			return flightSeed{}, fmt.Errorf("reload seed flight %s: %w", flightNo, err)
		}
	} else if err != nil {
		return flightSeed{}, fmt.Errorf("find seed flight %s: %w", flightNo, err)
	} else if row.PublicID == deterministicID(options.Prefix, options.Seed, "flight", index) {
		if err := tx.Table("flight").Where("id = ?", row.ID).Updates(values).Error; err != nil {
			return flightSeed{}, fmt.Errorf("update seed flight %s: %w", flightNo, err)
		}
		row.Status, row.DisplayNo, row.ScheduledAt = status, flightNo, scheduledAt
	}
	return flightSeed{ID: row.ID, PublicID: row.PublicID, DisplayNo: row.DisplayNo, ScheduledAt: row.ScheduledAt}, nil
}

type flightSeed struct {
	ID          uint64
	PublicID    string
	DisplayNo   string
	ScheduledAt time.Time
}

func ensureTaskGraph(tx *gorm.DB, options Options, index int, flight flightSeed, template templateSeed, candidates []PersonnelSeed, selected PersonnelSeed, status string) (string, string, error) {
	taskPublicID := deterministicID(options.Prefix, options.Seed, "task", index)
	generationKey := fmt.Sprintf("%s-%d-task-%04d", strings.ToLower(options.Prefix), options.Seed, index+1)
	plannedAt := flight.ScheduledAt.Add(time.Duration(template.OffsetSeconds) * time.Second)
	statusVersion := taskStatusVersion(status)
	syncVersion := uint64(index%5 + 1)
	taskValues := map[string]any{"public_id": taskPublicID, "flight_id": flight.ID, "template_id": template.ID, "area_id": template.AreaID, "team_id": template.TeamID, "trigger_type": "flight_arrived", "generation_key": generationKey, "source_event_id": deterministicID(options.Prefix, options.Seed, "source-event", index), "template_version": 100 + index%len(teamSpecs), "task_name": template.Name, "message": template.Message, "planned_at": plannedAt, "status": status, "status_version": statusVersion, "sync_version": syncVersion, "updated_at": options.Now}
	var taskRow struct {
		ID       uint64
		PublicID string
	}
	err := tx.Table("task_instance").Where("generation_key = ?", generationKey).First(&taskRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Table("task_instance").Create(taskValues).Error; err != nil {
			return "", "", fmt.Errorf("create seed task %s: %w", template.Name, err)
		}
		if err := tx.Table("task_instance").Where("generation_key = ?", generationKey).First(&taskRow).Error; err != nil {
			return "", "", fmt.Errorf("reload seed task %s: %w", template.Name, err)
		}
	} else if err != nil {
		return "", "", fmt.Errorf("find seed task %s: %w", template.Name, err)
	} else if taskRow.PublicID == taskPublicID {
		if err := tx.Table("task_instance").Where("id = ?", taskRow.ID).Updates(taskValues).Error; err != nil {
			return "", "", fmt.Errorf("update seed task %s: %w", template.Name, err)
		}
	}
	// A seed version may change which team owns a task. That can make the
	// desired personnel overlap with an older candidate row while the stable
	// candidate public IDs remain the same. Stage only those deterministic seed
	// rows on otherwise-unused personnel IDs so the (task_id, personnel_id)
	// unique key remains valid without deleting any candidate history.
	seedCandidateIDs := make([]string, 0, len(candidates))
	desiredPersonnelIDs := make([]uint64, 0, len(candidates))
	for rank := range candidates {
		seedCandidateIDs = append(seedCandidateIDs, deterministicID(options.Prefix, options.Seed, "candidate", index*3+rank))
		desiredPersonnelIDs = append(desiredPersonnelIDs, candidates[rank].ID)
	}
	var existingSeedCandidates []struct {
		ID uint64
	}
	if err := tx.Table("task_candidate").Where("task_id = ? AND public_id IN ?", taskRow.ID, seedCandidateIDs).Find(&existingSeedCandidates).Error; err != nil {
		return "", "", fmt.Errorf("find existing seed candidates: %w", err)
	}
	if len(existingSeedCandidates) > 0 {
		var temporaryPersonnelIDs []uint64
		if err := tx.Table("personnel").Select("id").Where("id NOT IN (SELECT personnel_id FROM task_candidate WHERE task_id = ?) AND id NOT IN ?", taskRow.ID, desiredPersonnelIDs).Limit(len(existingSeedCandidates)).Find(&temporaryPersonnelIDs).Error; err != nil {
			return "", "", fmt.Errorf("find temporary seed personnel: %w", err)
		}
		if len(temporaryPersonnelIDs) != len(existingSeedCandidates) {
			return "", "", fmt.Errorf("not enough temporary personnel for task %s candidates", taskPublicID)
		}
		for i, candidate := range existingSeedCandidates {
			if err := tx.Table("task_candidate").Where("id = ?", candidate.ID).Updates(map[string]any{"personnel_id": temporaryPersonnelIDs[i]}).Error; err != nil {
				return "", "", fmt.Errorf("stage seed candidate: %w", err)
			}
		}
	}
	for rank, candidate := range candidates {
		candidateStatus := "proposed"
		if status != "pending_dispatch" {
			candidateStatus = "rejected"
			if candidate.ID == selected.ID {
				candidateStatus = "selected"
			}
		}
		candidateValues := map[string]any{"public_id": deterministicID(options.Prefix, options.Seed, "candidate", index*3+rank), "task_id": taskRow.ID, "personnel_id": candidate.ID, "candidate_rank": rank + 1, "status": candidateStatus, "matched_position_code": candidate.Position, "matched_capabilities": mustJSON(candidate.Capabilities), "personnel_work_state_snapshot": "idle", "personnel_state_changed_at_snapshot": options.Now}
		var candidateID uint64
		// The candidate public ID is the seed's stable identity. Looking it up
		// first lets a later seed version safely remap an existing demo task to
		// a newly added service team without colliding with the unique key.
		if err := tx.Table("task_candidate").Where("public_id = ?", candidateValues["public_id"]).Select("id").Scan(&candidateID).Error; err != nil {
			return "", "", fmt.Errorf("find seed candidate: %w", err)
		}
		if candidateID == 0 {
			if err := tx.Table("task_candidate").Where("task_id = ? AND personnel_id = ?", taskRow.ID, candidate.ID).Select("id").Scan(&candidateID).Error; err != nil {
				return "", "", fmt.Errorf("find seed candidate by task/personnel: %w", err)
			}
		}
		if candidateID == 0 {
			if err := tx.Table("task_candidate").Create(candidateValues).Error; err != nil {
				return "", "", fmt.Errorf("create seed candidate: %w", err)
			}
		} else if err := tx.Table("task_candidate").Where("id = ?", candidateID).Updates(candidateValues).Error; err != nil {
			return "", "", fmt.Errorf("update seed candidate: %w", err)
		}
	}
	if err := ensureTaskHistory(tx, options, index, taskRow.ID, status); err != nil {
		return "", "", err
	}
	if status == "pending_dispatch" {
		return taskRow.PublicID, "", nil
	}
	var candidateRow struct {
		ID       uint64
		PublicID string
	}
	if err := tx.Table("task_candidate").Where("task_id = ? AND personnel_id = ?", taskRow.ID, selected.ID).First(&candidateRow).Error; err != nil {
		return "", "", fmt.Errorf("find selected seed candidate: %w", err)
	}
	assignmentPublicID := deterministicID(options.Prefix, options.Seed, "assignment", index)
	assignmentStatus := assignmentStatusForTask(status)
	confirmedAt := options.Now.Add(-time.Duration(index%8+1) * time.Hour)
	assignmentValues := map[string]any{"public_id": assignmentPublicID, "task_id": taskRow.ID, "candidate_id": candidateRow.ID, "personnel_id": selected.ID, "status": assignmentStatus, "status_version": assignmentStatusVersion(assignmentStatus), "confirmation_id": fmt.Sprintf("%s-confirm-%04d", strings.ToLower(options.Prefix), index+1), "confirmed_by_public_id": deterministicID(options.Prefix, options.Seed, "admin-identity", 2), "confirmed_at": confirmedAt, "accepted_at": optionalSeedTime(assignmentStatus, "accepted", confirmedAt.Add(5*time.Minute)), "completed_at": optionalSeedTime(assignmentStatus, "completed", confirmedAt.Add(35*time.Minute)), "cancelled_at": optionalSeedTime(assignmentStatus, "cancelled", confirmedAt.Add(10*time.Minute)), "cancellation_id": optionalSeedString(assignmentStatus, "cancelled", fmt.Sprintf("%s-cancel-%04d", strings.ToLower(options.Prefix), index+1)), "cancel_reason": optionalSeedString(assignmentStatus, "cancelled", "演示数据：航班服务状态调整")}
	var assignmentID uint64
	if err := tx.Table("task_assignment").Where("task_id = ?", taskRow.ID).Select("id").Scan(&assignmentID).Error; err != nil {
		return "", "", fmt.Errorf("find seed assignment: %w", err)
	}
	if assignmentID == 0 {
		if err := tx.Table("task_assignment").Create(assignmentValues).Error; err != nil {
			return "", "", fmt.Errorf("create seed assignment: %w", err)
		}
		if err := tx.Table("task_assignment").Where("task_id = ?", taskRow.ID).Select("id").Scan(&assignmentID).Error; err != nil {
			return "", "", fmt.Errorf("reload seed assignment: %w", err)
		}
	} else if err := tx.Table("task_assignment").Where("id = ?", assignmentID).Updates(assignmentValues).Error; err != nil {
		return "", "", fmt.Errorf("update seed assignment: %w", err)
	}
	if err := ensureAssignmentHistory(tx, options, index, assignmentID, assignmentStatus); err != nil {
		return "", "", err
	}
	return taskRow.PublicID, assignmentPublicID, nil
}

func ensureTaskHistory(tx *gorm.DB, options Options, index int, taskID uint64, status string) error {
	steps := []string{"pending_dispatch"}
	switch status {
	case "assigned":
		steps = append(steps, "assigned")
	case "in_progress":
		steps = append(steps, "assigned", "in_progress")
	case "completed":
		steps = append(steps, "assigned", "in_progress", "completed")
	case "cancelled":
		steps = append(steps, "assigned", "cancelled")
	}
	for version, toStatus := range steps {
		publicID := deterministicID(options.Prefix, options.Seed, "task-history", index*4+version)
		var id uint64
		if err := tx.Table("task_status_history").Where("public_id = ?", publicID).Select("id").Scan(&id).Error; err != nil {
			return fmt.Errorf("find seed task history: %w", err)
		}
		if id != 0 {
			continue
		}
		var from any
		if version > 0 {
			from = steps[version-1]
		}
		values := map[string]any{"public_id": publicID, "task_id": taskID, "status_version": version, "from_status": from, "to_status": toStatus, "reason": "演示数据", "actor_type": "machine", "actor_public_id": deterministicID(options.Prefix, options.Seed, "seed-actor", 0), "occurred_at": options.Now.Add(-time.Duration(len(steps)-version) * time.Hour)}
		if err := tx.Table("task_status_history").Create(values).Error; err != nil {
			return fmt.Errorf("create seed task history: %w", err)
		}
	}
	return nil
}

func ensureAssignmentHistory(tx *gorm.DB, options Options, index int, assignmentID uint64, status string) error {
	steps := []string{"confirmed"}
	switch status {
	case "accepted":
		steps = append(steps, "accepted")
	case "completed":
		steps = append(steps, "accepted", "completed")
	case "cancelled":
		steps = append(steps, "cancelled")
	}
	for version, toStatus := range steps {
		publicID := deterministicID(options.Prefix, options.Seed, "assignment-history", index*3+version)
		var id uint64
		if err := tx.Table("task_assignment_status_history").Where("public_id = ?", publicID).Select("id").Scan(&id).Error; err != nil {
			return fmt.Errorf("find seed assignment history: %w", err)
		}
		if id != 0 {
			continue
		}
		var from any
		if version > 0 {
			from = steps[version-1]
		}
		values := map[string]any{"public_id": publicID, "assignment_id": assignmentID, "status_version": version, "from_status": from, "to_status": toStatus, "reason": "演示数据", "actor_type": "machine", "actor_public_id": deterministicID(options.Prefix, options.Seed, "seed-actor", 0), "occurred_at": options.Now.Add(-time.Duration(len(steps)-version) * time.Hour)}
		if err := tx.Table("task_assignment_status_history").Create(values).Error; err != nil {
			return fmt.Errorf("create seed assignment history: %w", err)
		}
	}
	return nil
}

func ensureSeedOutbox(tx *gorm.DB, options Options, projection ProjectionSeed) error {
	eventType, err := seedOutboxEventType(projection.Status)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"task_public_id": projection.PublicID, "assignment_public_id": projection.AssignmentPublicID, "employee_public_id": projection.EmployeePublicID, "flight_display_no": projection.FlightDisplayNo, "task_name": projection.TaskName, "area_name": projection.AreaName, "planned_at": projection.PlannedAt.UTC(), "business_status": projection.Status, "message": projection.Message, "sync_version": projection.SyncVersion})
	if err != nil {
		return fmt.Errorf("marshal seed outbox payload: %w", err)
	}
	eventID := deterministicID(options.Prefix, options.Seed, "outbox-"+projection.PublicID, 0)
	var row struct {
		ID        uint64
		EventType string
		Status    string
	}
	if err := tx.Table("outbox_event").Where("event_id = ?", eventID).Select("id, event_type, status").Scan(&row).Error; err != nil {
		return fmt.Errorf("find seed outbox: %w", err)
	}
	if row.ID != 0 {
		// Repair old demo rows created before the lifecycle event mapping was
		// complete. Re-queueing only a mismatched/failed seed event is safe:
		// Edge Inbox deduplicates the event ID and will re-run a previously
		// failed projection with the corrected event type.
		if row.EventType != eventType || row.Status == "failed" {
			if err := tx.Table("outbox_event").Where("id = ?", row.ID).Updates(map[string]any{
				"event_type": eventType, "payload": payload, "status": "pending", "attempts": 0,
				"next_attempt_at": options.Now, "last_error": nil, "lease_owner": nil, "lease_expires_at": nil,
				"updated_at": options.Now,
			}).Error; err != nil {
				return fmt.Errorf("repair seed outbox %s: %w", eventID, err)
			}
		}
		return nil
	}
	now := options.Now
	values := map[string]any{"event_id": eventID, "event_type": eventType, "schema_version": 1, "aggregate_type": "task", "aggregate_id": projection.PublicID, "occurred_at": projection.PlannedAt.UTC(), "producer": "dev-seed", "correlation_id": deterministicID(options.Prefix, options.Seed, "correlation", int(projection.SyncVersion)+len(projection.PublicID)), "trace_id": fmt.Sprintf("%s-seed-%d", strings.ToLower(options.Prefix), options.Seed), "payload": payload, "status": "pending", "attempts": 0, "next_attempt_at": now, "created_at": now, "updated_at": now}
	if err := tx.Table("outbox_event").Create(values).Error; err != nil {
		return fmt.Errorf("create seed outbox: %w", err)
	}
	return nil
}

func peopleForTeam(personnel []PersonnelSeed, teamID uint64) []PersonnelSeed {
	result := make([]PersonnelSeed, 0)
	for _, person := range personnel {
		if person.TeamID == teamID {
			result = append(result, person)
		}
	}
	return result
}

// Active demo assignments must not overlap on one employee. Otherwise a later
// historical task can leave the employee idle while an earlier assigned task
// still expects the reserved state, making the seeded lifecycle impossible to
// exercise through the real command path.
func selectSeedPersonnel(teamPeople []PersonnelSeed, status string, index int, states map[uint64]string) (PersonnelSeed, error) {
	if len(teamPeople) == 0 {
		return PersonnelSeed{}, errors.New("cannot select seeded personnel from an empty team")
	}
	for offset := 0; offset < len(teamPeople); offset++ {
		person := teamPeople[(index+offset)%len(teamPeople)]
		if status != "assigned" && status != "in_progress" {
			return person, nil
		}
		if states[person.ID] == "idle" {
			return person, nil
		}
	}
	return PersonnelSeed{}, fmt.Errorf("team has no available personnel for seeded %s task", status)
}

func seedCandidatePool(teamPeople []PersonnelSeed, selected PersonnelSeed, start int) []PersonnelSeed {
	result := make([]PersonnelSeed, 0, minInt(3, len(teamPeople)))
	result = append(result, selected)
	for offset := 0; offset < len(teamPeople) && len(result) < 3; offset++ {
		candidate := teamPeople[(start+offset)%len(teamPeople)]
		if candidate.ID == selected.ID {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func mergePersonnelSeedState(current, desired string) string {
	if personnelSeedStateRank(desired) > personnelSeedStateRank(current) {
		return desired
	}
	return current
}

func personnelSeedStateRank(state string) int {
	switch state {
	case "busy":
		return 2
	case "reserved":
		return 1
	default:
		return 0
	}
}

func seedOutboxEventType(status string) (string, error) {
	switch status {
	case "assigned":
		return "task.assigned.v1", nil
	case "in_progress":
		return "task.started.v1", nil
	case "completed":
		return "task.completed.v1", nil
	case "cancelled":
		return "task.cancelled.v1", nil
	default:
		return "", fmt.Errorf("unsupported seeded projection status %q", status)
	}
}

func seededTaskStatus(index int) string {
	return []string{"pending_dispatch", "assigned", "in_progress", "completed", "cancelled"}[index%5]
}

func personnelStateForTask(status string) string {
	switch status {
	case "assigned":
		return "reserved"
	case "in_progress":
		return "busy"
	default:
		return "idle"
	}
}

func assignmentStatusForTask(status string) string {
	switch status {
	case "in_progress":
		return "accepted"
	case "completed":
		return "completed"
	case "cancelled":
		return "cancelled"
	default:
		return "confirmed"
	}
}

func taskStatusVersion(status string) uint64 {
	switch status {
	case "assigned":
		return 1
	case "in_progress", "cancelled":
		return 2
	case "completed":
		return 3
	default:
		return 0
	}
}

func assignmentStatusVersion(status string) uint64 {
	switch status {
	case "accepted":
		return 1
	case "completed":
		return 2
	case "cancelled":
		return 1
	default:
		return 0
	}
}

func optionalSeedTime(status, expected string, value time.Time) any {
	if status == expected || (expected == "accepted" && (status == "completed")) {
		return value
	}
	return nil
}

func optionalSeedString(status, expected, value string) any {
	if status == expected {
		return value
	}
	return nil
}

func mustJSON(value []string) []byte {
	result, _ := json.Marshal(value)
	return result
}

func seededName(index int) string {
	surnames := []string{"李", "王", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴", "徐", "孙"}
	given := []string{"晨", "宇", "宁", "悦", "航", "欣", "磊", "琳", "杰", "倩", "博", "敏"}
	return fmt.Sprintf("%s%s%02d", surnames[index%len(surnames)], given[(index/len(surnames))%len(given)], index+1)
}

func deterministicID(prefix string, seed int64, kind string, index int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s:%d", strings.ToLower(prefix), seed, kind, index)))
	digest[6] = (digest[6] & 0x0f) | 0x50
	digest[8] = (digest[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(digest[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func seedCode(prefix, value string) string {
	return strings.ToUpper(strings.TrimSpace(prefix)) + "-" + strings.ToUpper(strings.TrimSpace(value))
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// LoadManifestFromCore is used when only the Edge target is requested. It
// reads the seeded Core projection source but never lets Edge become a source
// of business truth.
func LoadManifestFromCore(ctx context.Context, db *gorm.DB, options Options) (Manifest, error) {
	if db == nil {
		return Manifest{}, errors.New("core database is nil")
	}
	options, err := options.normalized()
	if err != nil {
		return Manifest{}, err
	}
	var rows []struct {
		PublicID           string    `gorm:"column:public_id"`
		AssignmentPublicID string    `gorm:"column:assignment_public_id"`
		EmployeePublicID   string    `gorm:"column:employee_public_id"`
		FlightDisplayNo    string    `gorm:"column:flight_display_no"`
		TaskName           string    `gorm:"column:task_name"`
		AreaName           string    `gorm:"column:area_name"`
		PlannedAt          time.Time `gorm:"column:planned_at"`
		Status             string    `gorm:"column:status"`
		Message            string    `gorm:"column:message"`
		SyncVersion        uint64    `gorm:"column:sync_version"`
	}
	pattern := fmt.Sprintf("%s-%d-task-%%", strings.ToLower(options.Prefix), options.Seed)
	if err := db.WithContext(ctx).Table("task_instance AS ti").Select(`ti.public_id, a.public_id AS assignment_public_id, p.public_id AS employee_public_id, f.flight_display_no, ti.task_name, oa.name AS area_name, ti.planned_at, ti.status, ti.message, ti.sync_version`).Joins("JOIN task_assignment AS a ON a.task_id = ti.id").Joins("JOIN personnel AS p ON p.id = a.personnel_id").Joins("JOIN flight AS f ON f.id = ti.flight_id").Joins("JOIN operation_area AS oa ON oa.id = ti.area_id").Where("ti.generation_key LIKE ? AND ti.status NOT IN ?", pattern, []string{"pending_dispatch", "awaiting_confirmation"}).Order("ti.public_id ASC").Find(&rows).Error; err != nil {
		return Manifest{}, fmt.Errorf("load seeded Core projection source: %w", err)
	}
	if len(rows) == 0 {
		return Manifest{}, ErrNoSeedData
	}
	manifest := Manifest{Seed: options.Seed, Prefix: options.Prefix, Password: options.Password, Projections: make([]ProjectionSeed, 0, len(rows))}
	for _, row := range rows {
		manifest.Projections = append(manifest.Projections, ProjectionSeed{PublicID: row.PublicID, AssignmentPublicID: row.AssignmentPublicID, EmployeePublicID: row.EmployeePublicID, FlightDisplayNo: row.FlightDisplayNo, TaskName: row.TaskName, AreaName: row.AreaName, PlannedAt: row.PlannedAt, Status: row.Status, Message: row.Message, SyncVersion: row.SyncVersion})
	}
	return manifest, nil
}
