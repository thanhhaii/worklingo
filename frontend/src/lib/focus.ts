import type { LearningGoal, WorkContext } from "./types";
type FocusDefinition = { label: string; instruction: string; context: WorkContext; templates: {personaId:string;channel:string;title:string;message:string;points:string[]}[] };
export const FOCUS: Record<LearningGoal, FocusDefinition> = {
  "STATUS_UPDATE": {
    "label": "Status update",
    "instruction": "Report progress, blockers, and a clear next action.",
    "context": {
      "project": "HR Portal",
      "projectSummary": "Internal employee portal",
      "feature": "user management",
      "progress": "70% complete; roles remaining",
      "blocker": "Waiting on security rules",
      "notes": [
        "Friday demo"
      ]
    },
    "templates": [
      {
        "personaId": "sarah",
        "channel": "general",
        "title": "Share your progress",
        "message": "@{{username}} How is user management progressing for the Friday demo? Please reply in this task’s thread.",
        "points": [
          "Nêu tiến độ 70% và phần roles còn lại",
          "Nêu đang chờ security rules"
        ]
      },
      {
        "personaId": "alex",
        "channel": "general",
        "title": "Explain the blocker",
        "message": "@{{username}} What is blocking you, and what will you do next? Please reply in this task’s thread.",
        "points": [
          "Nêu đang chờ security rules",
          "Đề xuất bước tiếp theo phù hợp"
        ]
      },
      {
        "personaId": "sarah",
        "channel": "general",
        "title": "Stakeholder recap",
        "message": "@{{username}} Could you give stakeholders a concise recap for the Friday demo? Please reply in this task’s thread.",
        "points": [
          "Tóm tắt tiến độ 70% và roles còn lại",
          "Nêu blocker và bước tiếp theo"
        ]
      }
    ]
  },
  "CLARIFY_REQUIREMENTS": {
    "label": "Clarify requirements",
    "instruction": "Ask specific questions, summarize assumptions, and confirm requirements before committing.",
    "context": {
      "project": "ShopFlow",
      "projectSummary": "Online shopping platform",
      "feature": "CSV export",
      "progress": "40% complete",
      "blocker": "Premium export is undefined",
      "notes": [
        "CSV format, columns and date range not agreed",
        "No firm acceptance criteria"
      ]
    },
    "templates": [
      {
        "personaId": "david",
        "channel": "sprint-planning",
        "title": "Clarify premium export",
        "message": "@{{username}} Could you estimate the premium export? Please reply in this task’s thread.",
        "points": [
          "Hỏi premium export nghĩa là gì",
          "Chỉ ước lượng sau khi làm rõ yêu cầu"
        ]
      },
      {
        "personaId": "sarah",
        "channel": "sprint-planning",
        "title": "Clarify report scope",
        "message": "@{{username}} What scope should we include in the report? Please reply in this task’s thread.",
        "points": [
          "Hỏi các cột cần xuất",
          "Hỏi khoảng ngày cần hỗ trợ"
        ]
      },
      {
        "personaId": "alex",
        "channel": "sprint-planning",
        "title": "Confirm build assumptions",
        "message": "@{{username}} Are we ready to build the CSV export? Please reply in this task’s thread.",
        "points": [
          "Tóm tắt các giả định về CSV export",
          "Xin xác nhận tiêu chí nghiệm thu"
        ]
      }
    ]
  },
  "REQUEST_EXTENSION": {
    "label": "Request an extension",
    "instruction": "Request a realistic extension with a reason, revised date, and a useful alternative.",
    "context": {
      "project": "PayBridge",
      "projectSummary": "Payment processing platform",
      "feature": "payment integration",
      "progress": "60% complete; sandbox works",
      "blocker": "Production credentials missing",
      "notes": [
        "Refunds and tests remain",
        "Monday go-live; propose Wednesday"
      ]
    },
    "templates": [
      {
        "personaId": "sarah",
        "channel": "general",
        "title": "Request a revised deadline",
        "message": "@{{username}} Are we ready for the Monday go-live? Please reply in this task’s thread.",
        "points": [
          "Xin dời hạn sang Wednesday",
          "Nêu thiếu production credentials và còn refunds/tests"
        ]
      },
      {
        "personaId": "david",
        "channel": "general",
        "title": "Offer a partial delivery",
        "message": "@{{username}} Can you confirm delivery on Monday? Please reply in this task’s thread.",
        "points": [
          "Đề xuất demo sandbox vào Monday",
          "Đề xuất hoàn thành vào Wednesday"
        ]
      },
      {
        "personaId": "alex",
        "channel": "general",
        "title": "Make a realistic commitment",
        "message": "@{{username}} What can you realistically commit to, and when will you update us? Please reply in this task’s thread.",
        "points": [
          "Nêu phụ thuộc production credentials",
          "Đưa ra cam kết và thời điểm cập nhật tiếp"
        ]
      }
    ]
  },
  "POLITE_PUSHBACK": {
    "label": "Polite pushback",
    "instruction": "Push back professionally using concrete risks, tradeoffs, boundaries, and alternatives.",
    "context": {
      "project": "HR Portal",
      "projectSummary": "Internal employee portal",
      "feature": "user management",
      "progress": "70% complete",
      "blocker": "Mandatory role checks incomplete",
      "notes": [
        "Friday demo",
        "Client wants audit export added immediately",
        "Current sprint has no spare capacity"
      ]
    },
    "templates": [
      {
        "personaId": "sarah",
        "channel": "sprint-planning",
        "title": "Explain the scope tradeoff",
        "message": "@{{username}} Can we add audit export for Friday too? Please reply in this task’s thread.",
        "points": [
          "Nêu sprint không còn capacity",
          "Đề xuất đổi phạm vi hoặc giao theo giai đoạn"
        ]
      },
      {
        "personaId": "alex",
        "channel": "sprint-planning",
        "title": "Protect permission testing",
        "message": "@{{username}} Could we skip permission tests to make the demo? Please reply in this task’s thread.",
        "points": [
          "Nêu rủi ro thiếu mandatory role checks",
          "Đề xuất phương án an toàn hơn"
        ]
      },
      {
        "personaId": "david",
        "channel": "sprint-planning",
        "title": "Offer staged delivery",
        "message": "@{{username}} We need all the scope on Friday. Can you commit? Please reply in this task’s thread.",
        "points": [
          "Đặt giới hạn lịch sự với capacity hiện tại",
          "Đề xuất giao theo giai đoạn"
        ]
      }
    ]
  },
  "INCIDENT_UPDATE": {
    "label": "Incident update",
    "instruction": "Give a factual incident update: impact, investigation status, uncertainty, and the next update time. Never invent a resolution.",
    "context": {
      "project": "ShopFlow",
      "projectSummary": "Online shopping platform",
      "feature": "checkout incident",
      "progress": "Investigation ongoing",
      "blocker": "Database pool saturation suspected, not confirmed",
      "notes": [
        "502s affect 8% of requests since 10:42 UTC",
        "Next update 11:15 UTC"
      ]
    },
    "templates": [
      {
        "personaId": "alex",
        "channel": "incident-room",
        "title": "Initial incident update",
        "message": "@{{username}} Could you share an initial checkout incident update? Please reply in this task’s thread.",
        "points": [
          "Nêu 502 ảnh hưởng 8% requests từ 10:42 UTC",
          "Nêu đang điều tra và cập nhật lúc 11:15 UTC"
        ]
      },
      {
        "personaId": "sarah",
        "channel": "incident-room",
        "title": "Distinguish facts from suspicion",
        "message": "@{{username}} Do we know what caused the checkout errors? Please reply in this task’s thread.",
        "points": [
          "Nêu database pool saturation mới là nghi vấn",
          "Phân biệt thông tin xác nhận và đang điều tra"
        ]
      },
      {
        "personaId": "david",
        "channel": "incident-room",
        "title": "Customer incident summary",
        "message": "@{{username}} Could you summarize this for our customers? Please reply in this task’s thread.",
        "points": [
          "Thừa nhận ảnh hưởng một cách chuyên nghiệp",
          "Hẹn cập nhật 11:15 UTC, không tự nhận đã khắc phục"
        ]
      }
    ]
  }
};
export function goalLabel(goal: LearningGoal | null | undefined) { return goal ? FOCUS[goal]?.label ?? "Full simulation" : "Full simulation"; }
export function focusedLimit(durationMs:number) { return durationMs <= 600000 ? 2 : 3; }
export function focusedDue(start:number,expiry:number,step:number) {const d=expiry-start,first=Math.min(1500,d/20);return start+first+step*(d-first)/focusedLimit(d);}
