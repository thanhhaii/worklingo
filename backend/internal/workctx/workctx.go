// Package workctx generates the per-session Work context (fictional project situation).
package workctx

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
)

type Generator struct {
	llm   llm.Provider
	model string
	log   *slog.Logger
}

func NewGenerator(p llm.Provider, model string, log *slog.Logger) *Generator {
	return &Generator{llm: p, model: model, log: log}
}

const prompt = `You invent the shared work situation for a Business English Slack simulator at a fictional software company ("Acme Cloud").
The learner is a software engineer. Their colleagues are Sarah (product manager), Alex (tech lead) and David (a UK client).
Create ONE realistic "work context": a project, the feature the learner is currently building, how far along it is, one blocker, and 2-4 extra facts (demo date, a teammate on leave, tech stack, a client expectation...). Facts must be concrete and consistent (e.g. progress with a percentage).
Write every field in VIETNAMESE (proper names, product and technology names stay in English).
Respond with ONLY JSON:
{"project":"HR Portal","projectSummary":"1 sentence","feature":"...","progress":"...","blocker":"...","notes":["...","..."]}`

// Generate asks the LLM for a work context; on any failure it returns a static fallback.
func (g *Generator) Generate(ctx context.Context) dto.WorkContext {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var w dto.WorkContext
	temp := 0.9
	err := llm.CompleteJSON(cctx, g.llm, llm.Request{Model: g.model, MaxTokens: 700, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}}}, &w)
	if err == nil {
		w, err = Normalise(w)
	}
	if err != nil {
		g.log.Warn("work context generation failed, using fallback", "err", err)
		return Fallback()
	}
	return w
}

// Normalise trims fields and rejects incomplete contexts.
func Normalise(w dto.WorkContext) (dto.WorkContext, error) {
	w.Project = strings.TrimSpace(w.Project)
	w.ProjectSummary = strings.TrimSpace(w.ProjectSummary)
	w.Feature = strings.TrimSpace(w.Feature)
	w.Progress = strings.TrimSpace(w.Progress)
	w.Blocker = strings.TrimSpace(w.Blocker)
	notes := []string{}
	for _, n := range w.Notes {
		if n = strings.TrimSpace(n); n != "" {
			notes = append(notes, n)
		}
	}
	if len(notes) > 4 {
		notes = notes[:4]
	}
	w.Notes = notes
	if w.Project == "" || w.Feature == "" || w.Progress == "" || w.Blocker == "" {
		return w, fmt.Errorf("incomplete work context JSON")
	}
	return w, nil
}

var fallbacks = []dto.WorkContext{
	{
		Project:        "HR Portal",
		ProjectSummary: "Cổng nhân sự nội bộ để quản lý nhân viên, nghỉ phép và chấm công.",
		Feature:        "Tính năng quản lý user và phân quyền theo vai trò",
		Progress:       "Xong khoảng 70%: API và giao diện danh sách user đã xong, còn phần phân quyền",
		Blocker:        "Chưa có tài liệu về quy tắc phân quyền từ team bảo mật nên chưa chốt được thiết kế",
		Notes: []string{
			"Buổi demo cho khách hàng vào thứ Sáu tuần sau",
			"Đồng nghiệp làm frontend đang nghỉ phép đến thứ Tư",
			"Tech stack: Go, PostgreSQL và React",
		},
	},
	{
		Project:        "ShopFlow",
		ProjectSummary: "Nền tảng thương mại điện tử nhỏ cho các cửa hàng bán lẻ.",
		Feature:        "Tính năng xuất báo cáo đơn hàng ra file CSV",
		Progress:       "Xong khoảng 40%: đã có API lấy dữ liệu, chưa làm phần xuất file và giao diện",
		Blocker:        "Truy vấn báo cáo chạy chậm trên dữ liệu lớn, cần thêm index và kiểm tra lại",
		Notes: []string{
			"Khách hàng muốn có tính năng này trước cuối sprint",
			"Môi trường staging đang bị lỗi deploy từ hôm qua",
			"Tech stack: Node.js, PostgreSQL và Next.js",
		},
	},
	{
		Project:        "PayBridge",
		ProjectSummary: "Dịch vụ tích hợp cổng thanh toán cho các ứng dụng của khách hàng.",
		Feature:        "Tích hợp thanh toán qua ví điện tử và xử lý hoàn tiền",
		Progress:       "Xong khoảng 60%: luồng thanh toán đã chạy trên sandbox, còn phần hoàn tiền và test",
		Blocker:        "Đối tác thanh toán chưa cấp khóa API cho môi trường production",
		Notes: []string{
			"Ngày go-live dự kiến là thứ Hai tuần sau",
			"Thành viên QA duy nhất đang bận với dự án khác",
			"Tech stack: Go, Redis và PostgreSQL",
		},
	},
}

// Fallback returns a hand-written Vietnamese work context.
func Fallback() dto.WorkContext {
	return fallbacks[rand.IntN(len(fallbacks))]
}
