package testsupport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const FixturePassword = "fixture-password"

// CandidateScenario 是 B2 的最小候选匹配场景。
// 它包含一个合格员工、一个跨班组员工、一个能力不足员工和一个忙碌员工。
type CandidateScenario struct {
	Prefix            string
	Team              model.Team
	OtherTeam         model.Team
	Positions         []model.Position
	Users             []model.User
	TeamMembers       []model.TeamMember
	PersonnelStatuses []model.PersonnelStatus
	Flight            model.Flight
	TaskTemplate      model.TaskTemplate
}

// NewCandidateScenario 构造确定性的内存测试数据，不接触数据库。
func NewCandidateScenario(prefix string) (CandidateScenario, error) {
	prefix = normalizePrefix(prefix)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(FixturePassword), bcrypt.MinCost)
	if err != nil {
		return CandidateScenario{}, fmt.Errorf("生成测试密码失败: %w", err)
	}
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	team := model.Team{ID: 1001, Code: prefix + "-team-a", Name: prefix + "-班组 A", Enabled: true, CreatedAt: now, UpdatedAt: now}
	otherTeam := model.Team{ID: 1002, Code: prefix + "-team-b", Name: prefix + "-班组 B", Enabled: true, CreatedAt: now, UpdatedAt: now}
	positions := []model.Position{
		{ID: 1101, Category: "boarding", Name: prefix + "-登机口保障员", Capabilities: `["boarding","guidance"]`, Enabled: true, CreatedAt: now, UpdatedAt: now},
		{ID: 1102, Category: "checkin", Name: prefix + "-值机员", Capabilities: `["checkin"]`, Enabled: true, CreatedAt: now, UpdatedAt: now},
	}
	users := []model.User{
		{ID: 1201, Name: prefix + "-合格员工", EmployeeNo: prefix + "-eligible", Username: prefix + "-eligible", Password: string(passwordHash), Role: model.RoleStaff, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: 1202, Name: prefix + "-跨班组员工", EmployeeNo: prefix + "-wrong-team", Username: prefix + "-wrong-team", Password: string(passwordHash), Role: model.RoleStaff, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: 1203, Name: prefix + "-能力不足员工", EmployeeNo: prefix + "-wrong-capability", Username: prefix + "-wrong-capability", Password: string(passwordHash), Role: model.RoleStaff, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: 1204, Name: prefix + "-忙碌员工", EmployeeNo: prefix + "-busy", Username: prefix + "-busy", Password: string(passwordHash), Role: model.RoleStaff, Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	teamMembers := []model.TeamMember{
		{ID: 1301, TeamID: team.ID, UserID: users[0].ID, IsPrimary: true, CreatedAt: now},
		{ID: 1302, TeamID: otherTeam.ID, UserID: users[1].ID, IsPrimary: true, CreatedAt: now},
		{ID: 1303, TeamID: team.ID, UserID: users[2].ID, IsPrimary: false, CreatedAt: now},
		{ID: 1304, TeamID: team.ID, UserID: users[3].ID, IsPrimary: false, CreatedAt: now},
	}
	statuses := []model.PersonnelStatus{
		{ID: 1401, UserID: users[0].ID, Status: model.PersonnelIdle, LastEventTime: now.Add(-20 * time.Minute), Source: model.SourceTask, CreatedAt: now, UpdatedAt: now},
		{ID: 1402, UserID: users[1].ID, Status: model.PersonnelIdle, LastEventTime: now.Add(-30 * time.Minute), Source: model.SourceTask, CreatedAt: now, UpdatedAt: now},
		{ID: 1403, UserID: users[2].ID, Status: model.PersonnelIdle, LastEventTime: now.Add(-10 * time.Minute), Source: model.SourceTask, CreatedAt: now, UpdatedAt: now},
		{ID: 1404, UserID: users[3].ID, Status: model.PersonnelBusy, LastEventTime: now.Add(-5 * time.Minute), Source: model.SourceTask, CreatedAt: now, UpdatedAt: now},
	}
	flight := model.Flight{
		ID: 1501, FlightNo: prefix + "-CA1234", AircraftType: "A320", Origin: "PEK", Destination: "SHA",
		PlannedDeparture: now.Add(-2 * time.Hour), PlannedArrival: now.Add(-30 * time.Minute), ActualArrival: timePtr(now.Add(-5 * time.Minute)),
		Status: "arrived", Source: "manual", CreatedAt: now, UpdatedAt: now,
	}
	template := model.TaskTemplate{
		ID: 1601, Name: prefix + "-到达后保障", Phase: "arrival", RequiredPosition: "boarding",
		RequiredCapability: "boarding", TriggerEventType: "flight_arrived", RequiredCount: 1,
		TimeoutSeconds: 1800, WarningAdvanceSeconds: 300, Version: 1, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	return CandidateScenario{
		Prefix: prefix, Team: team, OtherTeam: otherTeam, Positions: positions, Users: users,
		TeamMembers: teamMembers, PersonnelStatuses: statuses, Flight: flight, TaskTemplate: template,
	}, nil
}

// Persist 显式将夹具写入数据库；调用方必须使用隔离测试数据库或唯一 Prefix。
func (s *CandidateScenario) Persist(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("测试夹具数据库不能为空")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		oldTeamAID := s.Team.ID
		oldTeamBID := s.OtherTeam.ID
		oldTeamIDs := map[int64]int64{oldTeamAID: 0, oldTeamBID: 0}
		oldUserIDs := make(map[int64]int64, len(s.Users))
		for i := range s.Users {
			oldUserIDs[s.Users[i].ID] = 0
		}

		s.Team.ID = 0
		if err := tx.Create(&s.Team).Error; err != nil {
			return err
		}
		oldTeamIDs[oldTeamAID] = s.Team.ID
		s.OtherTeam.ID = 0
		if err := tx.Create(&s.OtherTeam).Error; err != nil {
			return err
		}
		oldTeamIDs[oldTeamBID] = s.OtherTeam.ID
		for i := range s.Positions {
			s.Positions[i].ID = 0
			if err := tx.Create(&s.Positions[i]).Error; err != nil {
				return err
			}
		}
		for i := range s.Users {
			oldID := s.Users[i].ID
			s.Users[i].ID = 0
			if err := tx.Create(&s.Users[i]).Error; err != nil {
				return err
			}
			oldUserIDs[oldID] = s.Users[i].ID
		}
		for i := range s.TeamMembers {
			s.TeamMembers[i].ID = 0
			s.TeamMembers[i].TeamID = oldTeamIDs[s.TeamMembers[i].TeamID]
			s.TeamMembers[i].UserID = oldUserIDs[s.TeamMembers[i].UserID]
			if err := tx.Create(&s.TeamMembers[i]).Error; err != nil {
				return err
			}
		}
		for i := range s.PersonnelStatuses {
			s.PersonnelStatuses[i].ID = 0
			s.PersonnelStatuses[i].UserID = oldUserIDs[s.PersonnelStatuses[i].UserID]
			if err := tx.Create(&s.PersonnelStatuses[i]).Error; err != nil {
				return err
			}
		}
		s.Flight.ID = 0
		if err := tx.Create(&s.Flight).Error; err != nil {
			return err
		}
		s.TaskTemplate.ID = 0
		return tx.Create(&s.TaskTemplate).Error
	})
}

// Cleanup 只删除当前夹具已创建的主键记录，按依赖反向执行，不使用 TRUNCATE。
func (s CandidateScenario) Cleanup(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("测试夹具数据库不能为空")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleteByIDs(tx, &model.PersonnelStatus{}, idsOfStatuses(s.PersonnelStatuses)); err != nil {
			return err
		}
		if err := deleteByIDs(tx, &model.TeamMember{}, idsOfTeamMembers(s.TeamMembers)); err != nil {
			return err
		}
		if err := deleteByIDs(tx, &model.TaskTemplate{}, []int64{s.TaskTemplate.ID}); err != nil {
			return err
		}
		if err := deleteByIDs(tx, &model.Flight{}, []int64{s.Flight.ID}); err != nil {
			return err
		}
		if err := deleteByIDs(tx, &model.User{}, idsOfUsers(s.Users)); err != nil {
			return err
		}
		if err := deleteByIDs(tx, &model.Position{}, idsOfPositions(s.Positions)); err != nil {
			return err
		}
		return deleteByIDs(tx, &model.Team{}, []int64{s.Team.ID, s.OtherTeam.ID})
	})
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "fixture"
	}
	return prefix
}

func timePtr(value time.Time) *time.Time { return &value }

func deleteByIDs(tx *gorm.DB, value interface{}, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Unscoped().Where("id IN ?", ids).Delete(value).Error
}

func idsOfStatuses(values []model.PersonnelStatus) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
func idsOfTeamMembers(values []model.TeamMember) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
func idsOfUsers(values []model.User) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
func idsOfPositions(values []model.Position) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
