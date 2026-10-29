// Package focus defines deterministic writing practice; it never calls a provider.
package focus

import (
	"time"

	"github.com/worklingo/backend/internal/dto"
)

type Template struct {
	PersonaID, Channel, Title, Message string
	Points                             []string
}
type definition struct {
	instruction string
	context     dto.WorkContext
	templates   []Template
}

var definitions = map[string]definition{
	"STATUS_UPDATE": {instruction: "Report progress, blockers, and a clear next action.", context: dto.WorkContext{Project: "HR Portal", ProjectSummary: "Internal employee portal", Feature: "user management", Progress: "70% complete; roles remaining", Blocker: "Waiting on security rules", Notes: []string{"Friday demo"}}, templates: []Template{
		{PersonaID: "sarah", Channel: "general", Title: "Share your progress", Message: "@{{username}} How is user management progressing for the Friday demo? Please reply in this task’s thread.", Points: []string{"Nêu tiến độ 70% và phần roles còn lại", "Nêu đang chờ security rules"}},
		{PersonaID: "alex", Channel: "general", Title: "Explain the blocker", Message: "@{{username}} What is blocking you, and what will you do next? Please reply in this task’s thread.", Points: []string{"Nêu đang chờ security rules", "Đề xuất bước tiếp theo phù hợp"}},
		{PersonaID: "sarah", Channel: "general", Title: "Stakeholder recap", Message: "@{{username}} Could you give stakeholders a concise recap for the Friday demo? Please reply in this task’s thread.", Points: []string{"Tóm tắt tiến độ 70% và roles còn lại", "Nêu blocker và bước tiếp theo"}},
	}},
	"CLARIFY_REQUIREMENTS": {instruction: "Ask specific questions, summarize assumptions, and confirm requirements before committing.", context: dto.WorkContext{Project: "ShopFlow", ProjectSummary: "Online shopping platform", Feature: "CSV export", Progress: "40% complete", Blocker: "Premium export is undefined", Notes: []string{"CSV format, columns and date range not agreed", "No firm acceptance criteria"}}, templates: []Template{
		{PersonaID: "david", Channel: "sprint-planning", Title: "Clarify premium export", Message: "@{{username}} Could you estimate the premium export? Please reply in this task’s thread.", Points: []string{"Hỏi premium export nghĩa là gì", "Chỉ ước lượng sau khi làm rõ yêu cầu"}},
		{PersonaID: "sarah", Channel: "sprint-planning", Title: "Clarify report scope", Message: "@{{username}} What scope should we include in the report? Please reply in this task’s thread.", Points: []string{"Hỏi các cột cần xuất", "Hỏi khoảng ngày cần hỗ trợ"}},
		{PersonaID: "alex", Channel: "sprint-planning", Title: "Confirm build assumptions", Message: "@{{username}} Are we ready to build the CSV export? Please reply in this task’s thread.", Points: []string{"Tóm tắt các giả định về CSV export", "Xin xác nhận tiêu chí nghiệm thu"}},
	}},
	"REQUEST_EXTENSION": {instruction: "Request a realistic extension with a reason, revised date, and a useful alternative.", context: dto.WorkContext{Project: "PayBridge", ProjectSummary: "Payment processing platform", Feature: "payment integration", Progress: "60% complete; sandbox works", Blocker: "Production credentials missing", Notes: []string{"Refunds and tests remain", "Monday go-live; propose Wednesday"}}, templates: []Template{
		{PersonaID: "sarah", Channel: "general", Title: "Request a revised deadline", Message: "@{{username}} Are we ready for the Monday go-live? Please reply in this task’s thread.", Points: []string{"Xin dời hạn sang Wednesday", "Nêu thiếu production credentials và còn refunds/tests"}},
		{PersonaID: "david", Channel: "general", Title: "Offer a partial delivery", Message: "@{{username}} Can you confirm delivery on Monday? Please reply in this task’s thread.", Points: []string{"Đề xuất demo sandbox vào Monday", "Đề xuất hoàn thành vào Wednesday"}},
		{PersonaID: "alex", Channel: "general", Title: "Make a realistic commitment", Message: "@{{username}} What can you realistically commit to, and when will you update us? Please reply in this task’s thread.", Points: []string{"Nêu phụ thuộc production credentials", "Đưa ra cam kết và thời điểm cập nhật tiếp"}},
	}},
	"POLITE_PUSHBACK": {instruction: "Push back professionally using concrete risks, tradeoffs, boundaries, and alternatives.", context: dto.WorkContext{Project: "HR Portal", ProjectSummary: "Internal employee portal", Feature: "user management", Progress: "70% complete", Blocker: "Mandatory role checks incomplete", Notes: []string{"Friday demo", "Client wants audit export added immediately", "Current sprint has no spare capacity"}}, templates: []Template{
		{PersonaID: "sarah", Channel: "sprint-planning", Title: "Explain the scope tradeoff", Message: "@{{username}} Can we add audit export for Friday too? Please reply in this task’s thread.", Points: []string{"Nêu sprint không còn capacity", "Đề xuất đổi phạm vi hoặc giao theo giai đoạn"}},
		{PersonaID: "alex", Channel: "sprint-planning", Title: "Protect permission testing", Message: "@{{username}} Could we skip permission tests to make the demo? Please reply in this task’s thread.", Points: []string{"Nêu rủi ro thiếu mandatory role checks", "Đề xuất phương án an toàn hơn"}},
		{PersonaID: "david", Channel: "sprint-planning", Title: "Offer staged delivery", Message: "@{{username}} We need all the scope on Friday. Can you commit? Please reply in this task’s thread.", Points: []string{"Đặt giới hạn lịch sự với capacity hiện tại", "Đề xuất giao theo giai đoạn"}},
	}},
	"INCIDENT_UPDATE": {instruction: "Give a factual incident update: impact, investigation status, uncertainty, and the next update time. Never invent a resolution.", context: dto.WorkContext{Project: "ShopFlow", ProjectSummary: "Online shopping platform", Feature: "checkout incident", Progress: "Investigation ongoing", Blocker: "Database pool saturation suspected, not confirmed", Notes: []string{"502s affect 8% of requests since 10:42 UTC", "Next update 11:15 UTC"}}, templates: []Template{
		{PersonaID: "alex", Channel: "incident-room", Title: "Initial incident update", Message: "@{{username}} Could you share an initial checkout incident update? Please reply in this task’s thread.", Points: []string{"Nêu 502 ảnh hưởng 8% requests từ 10:42 UTC", "Nêu đang điều tra và cập nhật lúc 11:15 UTC"}},
		{PersonaID: "sarah", Channel: "incident-room", Title: "Distinguish facts from suspicion", Message: "@{{username}} Do we know what caused the checkout errors? Please reply in this task’s thread.", Points: []string{"Nêu database pool saturation mới là nghi vấn", "Phân biệt thông tin xác nhận và đang điều tra"}},
		{PersonaID: "david", Channel: "incident-room", Title: "Customer incident summary", Message: "@{{username}} Could you summarize this for our customers? Please reply in this task’s thread.", Points: []string{"Thừa nhận ảnh hưởng một cách chuyên nghiệp", "Hẹn cập nhật 11:15 UTC, không tự nhận đã khắc phục"}},
	}},
}

func Valid(goal string) bool { _, ok := definitions[goal]; return ok }
func Instructions(goal string) string {
	if !Valid(goal) {
		return ""
	}
	return "Focused writing goal: " + goal + ". " + definitions[goal].instruction + " Keep replies to 1–3 sentences and at most one follow-up. Use only the supplied facts; do not introduce unrelated tasks. No response deadlines or ghosting penalties. Professional disagreement and clarification questions are appropriate. Assess progress toward this communicative goal through grammar, tone, and vocabulary."
}
func Context(goal string) dto.WorkContext { return definitions[goal].context }
func Templates(goal string) []Template    { return definitions[goal].templates }
func TaskLimit(duration time.Duration) int {
	if duration <= 10*time.Minute {
		return 2
	}
	return 3
}
func FirstOffset(duration time.Duration) time.Duration {
	return min(1500*time.Millisecond, duration/20)
}
func Due(start, expiry time.Time, step int) time.Time {
	d := expiry.Sub(start)
	first := FirstOffset(d)
	return start.Add(first + time.Duration(step)*(d-first)/time.Duration(TaskLimit(d)))
}
