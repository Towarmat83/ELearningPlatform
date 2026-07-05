const API_BASE = "/api";

export class ApiError extends Error {
  body: Record<string, unknown> | null;
  constructor(
    public status: number,
    message: string,
    body?: Record<string, unknown>,
  ) {
    super(message);
    this.name = "ApiError";
    this.body = body ?? null;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  token?: string,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401 && token) {
    if (typeof localStorage !== "undefined") {
      localStorage.removeItem("token");
      localStorage.removeItem("user");
    }
    window.location.href = "/";
    throw new ApiError(401, "Session expirée");
  }

  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    let body: Record<string, unknown> | undefined = undefined;
    try {
      const err = await res.json();
      body = err;
      message = err.error || message;
    } catch {}
    throw new ApiError(res.status, message, body);
  }
  return res.json();
}

export const api = {
  get: <T>(path: string, token?: string) =>
    request<T>("GET", path, undefined, token),
  post: <T>(path: string, body: unknown, token?: string) =>
    request<T>("POST", path, body, token),
  put: <T>(path: string, body: unknown, token?: string) =>
    request<T>("PUT", path, body, token),
  delete: <T>(path: string, token?: string) =>
    request<T>("DELETE", path, undefined, token),
};

// ── Types ─────────────────────────────────────────────────────────────────────

export interface OAuthProvider {
  id: string;
  name: string;
}

export interface UserPublic {
  id: string;
  username: string;
  email: string;
  role: "admin" | "student";
  isActive: boolean;
  avatarUrl: string | null;
  bio: string | null;
  authProvider: string;
  createdAt: string;
}

export interface AuthResponse {
  token: string;
  user: UserPublic;
}

export interface CoursePrerequisite {
  course: string;
  minScore?: number;
  modules?: string[];
}

export interface Course {
  id: string;
  title: string;
  description: string | null;
  thumbnail: string | null;
  category: string | null;
  difficulty: string | null;
  isPublic: boolean;
  createdBy: string;
  creator_username: string | null;
  labCount: number;
  enrollmentCount: number;
  createdAt: string;
  updatedAt: string;
  prerequisites?: CoursePrerequisite[];
}

export interface MyCourse extends Course {
  completedLabs: number;
  totalScore: number;
}

export interface AdminUser extends UserPublic {
  enrolledCourses: number;
  completedLabs: number;
}

export interface UpdateUser {
  username?: string;
  bio?: string;
  avatarUrl?: string;
  isActive?: boolean;
  role?: string;
}

export interface PublicSettings {
  registration_enabled: string;
  sso_local_login_enabled: string;
  password_min_length: string;
  password_require_uppercase: string;
  password_require_number: string;
  oidc_enabled: string;
  ldap_enabled: string;
}

export interface Group {
  id: string;
  name: string;
  source: string;
  createdAt: string;
  memberCount: number;
  mappedRole: string;
}

export interface GroupMapping {
  groupName: string;
  platformRole: string;
}

export interface PlatformSetting {
  key: string;
  value: string;
  description: string | null;
}

export interface CourseEnrollment {
  userId: string;
  username: string;
  email: string;
  enrolledAt: string;
}

export interface LeaderboardEntry {
  id: string;
  username: string;
  email: string;
  avatarUrl: string | null;
  totalScore: number;
  passedModules: number;
  enrolledCourses: number;
}

export interface AdminStats {
  total_users: number;
  totalCourses: number;
  total_labs: number;
  total_submissions: number;
  total_enrollments: number;
  success_rate: string;
}

export interface Lab {
  id: string;
  courseId: string;
  title: string;
  description: string | null;
  labType: "form" | "ctf" | "interactive";
  moduleType?: "video" | "image" | "text" | "quiz";
  content: Record<string, unknown>;
  points: number;
  orderIndex: number;
  isPublished: boolean;
  hidden: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface LabProgress {
  completed: boolean;
  bestScore: number;
  total_attempts: number;
  completed_at: string | null;
}

export interface LabWithProgress {
  lab: Lab;
  progress: LabProgress | null;
}

export interface CourseProgress {
  courseId: string;
  userId: string;
  total_labs: number;
  completedLabs: number;
  total_points_possible: number;
  total_points_earned: number;
  completion_percentage: number;
  lab_progress: {
    lab_id: string;
    lab_title: string;
    labType: string;
    points: number;
    completed: boolean;
    bestScore: number;
    total_attempts: number;
    completed_at: string | null;
  }[];
}

export interface SubmissionResult {
  isCorrect: boolean;
  score: number;
  maxScore: number;
  feedback: string | null;
  questionResults?:
    | {
        questionId: string;
        isCorrect: boolean;
        pointsEarned: number;
        correctAnswer: string | null;
        explanation: string | null;
      }[]
    | null;
  flag_results?:
    | {
        flag_id: string;
        name: string;
        isCorrect: boolean;
        pointsEarned: number;
      }[]
    | null;
}

// ── Auth ──────────────────────────────────────────────────────────────────────

export const authApi = {
  login: (email: string, password: string) =>
    api.post<AuthResponse>("/auth/login", { email, password }),
  register: (username: string, email: string, password: string) =>
    api.post<AuthResponse>("/auth/register", { username, email, password }),
  me: (token: string) => api.get<UserPublic>("/auth/me", token),
  changePassword: (oldPassword: string, newPassword: string, token: string) =>
    api.put(
      "/auth/password",
      { oldPassword: oldPassword, newPassword: newPassword },
      token,
    ),
  updateProfile: (data: { bio?: string; avatarUrl?: string }, token: string) =>
    api.put<UserPublic>("/auth/profile", data, token),
};

export const ssoApi = {
  providers: () =>
    api.get<{ providers: OAuthProvider[] }>("/auth/oauth/providers"),
  authorize: (provider: string) =>
    api.get<{ url: string; state: string }>(
      `/auth/oauth/${provider}/authorize`,
    ),
  callback: (code: string, state: string) =>
    api.post<AuthResponse>("/auth/oauth/callback", { code, state }),
};

export const oidcApi = {
  authorize: () =>
    api.get<{ url: string; state: string }>("/auth/oidc/authorize"),
  callback: (code: string, state: string) =>
    api.post<AuthResponse>("/auth/oidc/callback", { code, state }),
};

export const ldapApi = {
  login: (email: string, password: string) =>
    api.post<AuthResponse>("/auth/ldap/login", { email, password }),
};

export const groupsApi = {
  list: (token: string) => api.get<{ groups: Group[] }>("/admin/groups", token),
  create: (name: string, token: string) =>
    api.post<{ id: string; message: string }>("/admin/groups", { name }, token),
  delete: (id: string, token: string) =>
    api.delete<{ message: string }>(`/admin/groups/${id}`, token),
  getMappings: (token: string) =>
    api.get<{ mappings: GroupMapping[] }>("/admin/groups/mappings", token),
  upsertMapping: (data: GroupMapping, token: string) =>
    api.post<{ message: string }>("/admin/groups/mappings", data, token),
  deleteMapping: (groupName: string, token: string) =>
    api.delete<{ message: string }>(
      `/admin/groups/mappings/${encodeURIComponent(groupName)}`,
      token,
    ),
};

export const publicSettingsApi = {
  get: () => api.get<PublicSettings>("/settings/public"),
};

export const adminSettingsApi = {
  get: (token: string) =>
    api.get<{ settings: PlatformSetting[] }>("/admin/settings", token),
  update: (data: Record<string, string>, token: string) =>
    api.put<{ settings: PlatformSetting[] }>("/admin/settings", data, token),
};

// ── Courses ───────────────────────────────────────────────────────────────────

export const coursesApi = {
  list: (params?: {
    category?: string;
    difficulty?: string;
    search?: string;
    page?: number;
    per_page?: number;
  }) => {
    const qs = params
      ? "?" +
        new URLSearchParams(
          Object.fromEntries(
            Object.entries(params)
              .filter(([_, v]) => v != null)
              .map(([k, v]) => [k, String(v)]),
          ),
        ).toString()
      : "";
    return api.get<{
      courses: Course[];
      total: number;
      page: number;
      per_page: number;
      total_pages: number;
    }>(`/courses${qs}`);
  },
  get: (id: string) => api.get<Course>(`/courses/${id}`),
  enroll: (id: string, token: string) =>
    api.post(`/courses/${id}/enroll`, {}, token),
  unenroll: (id: string, token: string) =>
    api.delete(`/courses/${id}/unenroll`, token),
  myCourses: (token: string) =>
    api.get<{ courses: MyCourse[] }>("/my/courses", token),
};

// ── Labs ──────────────────────────────────────────────────────────────────────

export const labsApi = {
  list: (courseId: string, token: string) =>
    api.get<{ labs: Lab[] }>(`/courses/${courseId}/labs`, token),
  get: (courseId: string, labId: string, token: string) =>
    api.get<LabWithProgress>(`/courses/${courseId}/labs/${labId}`, token),
  submit: (courseId: string, labId: string, answer: unknown, token: string) =>
    api.post<SubmissionResult>(
      `/courses/${courseId}/labs/${labId}/submit`,
      { answer },
      token,
    ),
  progress: (courseId: string, token: string) =>
    api.get<CourseProgress>(`/courses/${courseId}/progress`, token),
};

// ── Quiz Types ─────────────────────────────────────────────────────────────────

export interface QuizQuestion {
  id: string;
  type: "single" | "multiple" | "boolean" | "order";
  difficulty?: string;
  points: number;
  question: string;
  answers?: { id: string; text: string }[];
  items?: { id: string; text: string }[];
  sourceRefs?: SourceRef[];
}

export interface SourceRef {
  course: string;
  module: string;
  anchor: string;
  priority: number;
}

export interface QuizUserAnswer {
  single?: string;
  multiple?: string[];
  boolean?: boolean;
  order?: string[];
}

export interface QuizQuestionResult {
  questionId: string;
  type: string;
  isCorrect: boolean;
  pointsEarned: number;
  pointsMax: number;
  correctAnswer?: unknown;
  feedback?: string;
  sourceRefs?: SourceRef[];
}

export interface QuizCooldown {
  remainingSeconds: number;
  attempts: number;
  locked: boolean;
}

export interface QuizSubmitResponse {
  totalScore: number;
  maxScore: number;
  passed: boolean;
  questionResults: QuizQuestionResult[];
  cooldowns?: Record<string, QuizCooldown>;
}

export interface QuizCooldownError {
  error: string;
  cooldowns: Record<string, QuizCooldown>;
}

// ── Module API (quiz-type modules use these) ──────────────────────────────────────

export interface ModuleSummary {
  index: number;
  name: string;
  slug: string;
  type: "text" | "video" | "image" | "quiz" | "lab" | "modules";
  viewed: boolean;
  hidden: boolean;
  locked: boolean;
  prerequisites?: string[];
  bestScore: number;
  maxScore: number;
  passed: boolean;
  attempts: number;
  labUrl?: string;
  // Admin-only / git content
  src?: string;
  ref?: string;
  path?: string;
}

export interface ModuleQuizConfig {
  passingScore: number;
  cooldown?: {
    strategy: string;
    baseSeconds: number;
    multiplier: number;
    maxSeconds: number;
  };
  maxAttemptsPerQuestion: number | null;
  lockOnMaxAttempts: boolean;
}

export interface ModuleDetail {
  index: number;
  name: string;
  slug: string;
  type: "text" | "video" | "image" | "quiz" | "lab";
  content: string | null;
  viewed: boolean;
  hidden: boolean;
  hasCheck?: boolean;
  questions?: QuizQuestion[];
  quizConfig?: ModuleQuizConfig;
  cooldowns?: Record<string, QuizCooldown>;
}

export interface CheckResult {
  allow: boolean;
  violations: string[] | null;
}

export const lessonsApi = {
  markComplete: (courseSlug: string, lessonSlug: string, token: string) =>
    api.post<{ message: string }>(
      `/courses/${courseSlug}/lessons/${lessonSlug}/complete`,
      {},
      token,
    ),
};

export const modulesApi = {
  list: (courseSlug: string, token: string) =>
    api.get<{ modules: ModuleSummary[] }>(
      `/courses/${courseSlug}/modules`,
      token,
    ),
  get: (courseSlug: string, index: number, token: string) =>
    api.get<ModuleDetail>(`/courses/${courseSlug}/modules/${index}`, token),
  submit: (
    courseSlug: string,
    index: number,
    answers: Record<string, QuizUserAnswer>,
    token: string,
  ) =>
    api.post<QuizSubmitResponse>(
      `/courses/${courseSlug}/modules/${index}/submit`,
      { answers },
      token,
    ),
  check: (courseSlug: string, index: number, token: string) =>
    api.post<CheckResult>(
      `/courses/${courseSlug}/modules/${index}/check`,
      {},
      token,
    ),
};

// ── Admin ─────────────────────────────────────────────────────────────────────

export const adminApi = {
  clearCache: (token: string) =>
    api.post<{ status: string }>("/admin/cache/clear", {}, token),

  stats: (token: string) => api.get<AdminStats>("/admin/stats", token),
  leaderboard: (token: string) =>
    api.get<{ leaderboard: LeaderboardEntry[] }>("/admin/leaderboard", token),

  authProviders: (token: string) =>
    api.get<{ providers: { provider: string; count: number }[] }>(
      "/admin/users/providers",
      token,
    ),
  users: (token: string, provider?: string) =>
    api.get<{ users: AdminUser[]; total: number }>(
      provider
        ? `/admin/users?provider=${encodeURIComponent(provider)}`
        : "/admin/users",
      token,
    ),
  getUser: (id: string, token: string) =>
    api.get<AdminUser>(`/admin/users/${id}`, token),
  updateUser: (id: string, data: UpdateUser, token: string) =>
    api.put(`/admin/users/${id}`, data, token),
  deleteUser: (id: string, token: string) =>
    api.delete(`/admin/users/${id}`, token),

  adminCourses: (
    token: string,
    params?: { page?: number; per_page?: number; search?: string },
  ) => {
    const qs = params
      ? "?" +
        new URLSearchParams(
          Object.fromEntries(
            Object.entries(params)
              .filter(([_, v]) => v != null)
              .map(([k, v]) => [k, String(v)]),
          ),
        ).toString()
      : "";
    return api.get<{
      courses: Course[];
      total: number;
      page: number;
      per_page: number;
    }>(`/admin/courses${qs}`, token);
  },

  listEnrollments: (courseId: string, token: string) =>
    api.get<{ enrollments: CourseEnrollment[] }>(
      `/admin/courses/${courseId}/enrollments`,
      token,
    ),
  enrollUser: (courseId: string, userId: string, token: string) =>
    api.post(
      `/admin/courses/${courseId}/enrollments`,
      { userId: userId },
      token,
    ),
  unenrollUser: (courseId: string, userId: string, token: string) =>
    api.delete(`/admin/courses/${courseId}/enrollments/${userId}`, token),
  listGroupEnrollments: (courseId: string, token: string) =>
    api.get<{
      groups: {
        id: string;
        name: string;
        source: string;
        memberCount: number;
        enrolledAt: string;
      }[];
    }>(`/admin/courses/${courseId}/enrollments/groups`, token),
  enrollGroup: (courseId: string, groupId: string, token: string) =>
    api.post<{ message: string; enrolled: number }>(
      `/admin/courses/${courseId}/enrollments/groups`,
      { groupId: groupId },
      token,
    ),
  unenrollGroup: (courseId: string, groupId: string, token: string) =>
    api.delete(
      `/admin/courses/${courseId}/enrollments/groups/${groupId}`,
      token,
    ),

  clearCourseCache: (courseSlug: string, token: string) =>
    api.post<{ status: string; reposCleared: number }>(
      `/admin/courses/${courseSlug}/cache/clear`,
      {},
      token,
    ),
  clearModuleCache: (courseSlug: string, index: number, token: string) =>
    api.post<{ status: string; reposCleared: number }>(
      `/admin/courses/${courseSlug}/modules/${index}/cache/clear`,
      {},
      token,
    ),
};
