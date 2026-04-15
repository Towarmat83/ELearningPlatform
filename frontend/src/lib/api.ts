const API_BASE = '/api';

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  token?: string
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const err = await res.json();
      message = err.error || message;
    } catch {}
    throw new ApiError(res.status, message);
  }

  return res.json();
}

export const api = {
  get: <T>(path: string, token?: string) => request<T>('GET', path, undefined, token),
  post: <T>(path: string, body: unknown, token?: string) => request<T>('POST', path, body, token),
  put: <T>(path: string, body: unknown, token?: string) => request<T>('PUT', path, body, token),
  delete: <T>(path: string, token?: string) => request<T>('DELETE', path, undefined, token),
};

// ── Types ─────────────────────────────────────────────────────────────────────

export interface OAuthProvider {
  id: string;
  name: string;
}

// ── Auth ──────────────────────────────────────────────────────────────────────
export const authApi = {
  login: (email: string, password: string) =>
    api.post<AuthResponse>('/auth/login', { email, password }),
  register: (username: string, email: string, password: string) =>
    api.post<AuthResponse>('/auth/register', { username, email, password }),
  me: (token: string) => api.get<UserPublic>('/auth/me', token),
  changePassword: (oldPassword: string, newPassword: string, token: string) =>
    api.put('/auth/password', { old_password: oldPassword, new_password: newPassword }, token),
  updateProfile: (data: { bio?: string; avatar_url?: string }, token: string) =>
    api.put<UserPublic>('/auth/profile', data, token),
};

// SSO
export const ssoApi = {
  providers: () => api.get<{ providers: OAuthProvider[] }>('/auth/oauth/providers'),
  authorize: (provider: string) =>
    api.get<{ url: string; state: string }>(`/auth/oauth/${provider}/authorize`),
  callback: (code: string, state: string) =>
    api.post<AuthResponse>('/auth/oauth/callback', { code, state }),
};

// Public settings (subset exposed to unauthenticated users)
export interface PublicSettings {
  registration_enabled: string;     // "true" | "false"
  sso_local_login_enabled: string;  // "true" | "false"
  password_min_length: string;      // numeric string
  password_require_uppercase: string;
  password_require_number: string;
}

export const publicSettingsApi = {
  get: () => api.get<PublicSettings>('/settings/public'),
};

// Admin settings
export interface PlatformSetting {
  key: string;
  value: string;
  description: string | null;
}

export const adminSettingsApi = {
  get: (token: string) =>
    api.get<{ settings: PlatformSetting[] }>('/admin/settings', token),
  update: (data: Record<string, string>, token: string) =>
    api.put<{ settings: PlatformSetting[] }>('/admin/settings', data, token),
};

// Courses
export const coursesApi = {
  list: (params?: CourseFilter) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return api.get<CoursesResponse>(`/courses${qs}`);
  },
  get: (id: string) => api.get<CourseWithStats>(`/courses/${id}`),
  create: (data: CreateCourse, token: string) => api.post<Course>('/courses', data, token),
  update: (id: string, data: Partial<CreateCourse>, token: string) =>
    api.put<Course>(`/courses/${id}`, data, token),
  delete: (id: string, token: string) => api.delete(`/courses/${id}`, token),
  enroll: (id: string, token: string) => api.post(`/courses/${id}/enroll`, {}, token),
  unenroll: (id: string, token: string) => api.delete(`/courses/${id}/unenroll`, token),
  myCourses: (token: string) => api.get<{ courses: MyCourse[] }>('/my/courses', token),
};

// Labs
export const labsApi = {
  list: (courseId: string, token: string) =>
    api.get<{ labs: Lab[] }>(`/courses/${courseId}/labs`, token),
  get: (courseId: string, labId: string, token: string) =>
    api.get<{ lab: Lab; progress: LabProgress | null }>(`/courses/${courseId}/labs/${labId}`, token),
  create: (courseId: string, data: CreateLab, token: string) =>
    api.post<Lab>(`/courses/${courseId}/labs`, data, token),
  update: (courseId: string, labId: string, data: Partial<CreateLab>, token: string) =>
    api.put<Lab>(`/courses/${courseId}/labs/${labId}`, data, token),
  delete: (courseId: string, labId: string, token: string) =>
    api.delete(`/courses/${courseId}/labs/${labId}`, token),
  submit: (courseId: string, labId: string, answer: unknown, token: string) =>
    api.post<SubmissionResult>(`/courses/${courseId}/labs/${labId}/submit`, { answer }, token),
  mySubmissions: (courseId: string, labId: string, token: string) =>
    api.get<{ submissions: Submission[] }>(`/courses/${courseId}/labs/${labId}/submissions`, token),
  myProgress: (courseId: string, token: string) =>
    api.get<CourseProgress>(`/courses/${courseId}/progress`, token),
  leaderboard: (courseId: string, token: string) =>
    api.get<{ leaderboard: LeaderboardEntry[] }>(`/courses/${courseId}/leaderboard`, token),
};

// Lab instances (interactive environments)
export const instanceApi = {
  start: (courseId: string, labId: string, token: string) =>
    api.post<LabInstance>(`/courses/${courseId}/labs/${labId}/instance`, {}, token),
  get: (courseId: string, labId: string, token: string) =>
    api.get<LabInstance>(`/courses/${courseId}/labs/${labId}/instance`, token),
  stop: (courseId: string, labId: string, token: string) =>
    api.delete(`/courses/${courseId}/labs/${labId}/instance`, token),
};

// Admin
export const adminApi = {
  stats: (token: string) => api.get<AdminStats>('/admin/stats', token),
  users: (token: string) => api.get<{ users: AdminUser[]; total: number }>('/admin/users', token),
  getUser: (id: string, token: string) => api.get<AdminUser>(`/admin/users/${id}`, token),
  updateUser: (id: string, data: UpdateUser, token: string) =>
    api.put(`/admin/users/${id}`, data, token),
  deleteUser: (id: string, token: string) => api.delete(`/admin/users/${id}`, token),
  courses: (token: string) => api.get<{ courses: CourseWithStats[] }>('/admin/courses', token),
  courseMonitoring: (courseId: string, token: string) =>
    api.get<CourseMonitoring>(`/admin/courses/${courseId}/monitoring`, token),
  labSubmissions: (courseId: string, labId: string, token: string) =>
    api.get<{ submissions: AdminSubmission[] }>(
      `/admin/courses/${courseId}/labs/${labId}/submissions`,
      token
    ),
  adminGetLab: (courseId: string, labId: string, token: string) =>
    api.get<Lab>(`/admin/courses/${courseId}/labs/${labId}`, token),
  adminLabStats: (courseId: string, labId: string, token: string) =>
    api.get<LabStats>(`/admin/courses/${courseId}/labs/${labId}/stats`, token),
  courseEnrollments: (courseId: string, token: string) =>
    api.get<{ enrollments: AdminEnrollment[] }>(`/admin/courses/${courseId}/enrollments`, token),
  enrollUser: (courseId: string, userId: string, token: string) =>
    api.post(`/admin/courses/${courseId}/enrollments`, { user_id: userId }, token),
  unenrollUser: (courseId: string, userId: string, token: string) =>
    api.delete(`/admin/courses/${courseId}/enrollments/${userId}`, token),
};

// Types
export interface User {
  id: string;
  username: string;
  email: string;
  role: 'admin' | 'student';
}

export interface UserPublic extends User {
  avatar_url: string | null;
  bio: string | null;
  is_active: boolean;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: UserPublic;
}

export interface Course {
  id: string;
  title: string;
  description: string;
  thumbnail: string | null;
  category: string | null;
  difficulty: 'beginner' | 'intermediate' | 'advanced' | null;
  is_published: boolean;
  created_by: string;
  creator_username?: string;
  created_at: string;
  updated_at: string;
}

export interface CourseWithStats extends Course {
  lab_count: number;
  enrollment_count: number;
}

export interface MyCourse extends CourseWithStats {
  completed_labs: number;
  total_score: number;
}

export interface CoursesResponse {
  courses: CourseWithStats[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface CourseFilter {
  category?: string;
  difficulty?: string;
  search?: string;
  page?: number;
  per_page?: number;
}

export interface CreateCourse {
  title: string;
  description: string;
  thumbnail?: string;
  category?: string;
  difficulty?: string;
  is_published?: boolean;
}

export interface Lab {
  id: string;
  course_id: string;
  title: string;
  description: string;
  lab_type: 'form' | 'ctf' | 'interactive';
  content: LabContent;
  flag?: string; // admin only
  points: number;
  order_index: number;
  is_published: boolean;
  created_at: string;
}

export interface InteractiveCommand {
  cmd: string;
  explanation?: string;
}

export interface InteractiveStep {
  id: string;
  title: string;
  description: string; // markdown
  commands?: InteractiveCommand[];
}

export interface LabContent {
  // Single-flag CTF
  challenge?: string;
  category?: string;
  hints?: string[];
  resources?: { name: string; url: string }[];
  docker_image?: string;
  // Multi-flag CTF
  flags?: CtfFlag[];
  instructions?: string;
  flag_hint?: string;
  // Form
  questions?: Question[];
  // Interactive
  steps?: InteractiveStep[];
}

export interface CtfFlag {
  id: string;
  name: string;
  description: string;
  points: number;
}

export interface Question {
  id: string;
  text: string;
  type: 'multiple_choice' | 'text' | 'code';
  options?: string[];
  correct_answer?: string; // admin only
  points: number;
  explanation?: string;
}

export interface CreateLab {
  title: string;
  description: string;
  lab_type: 'form' | 'ctf' | 'interactive';
  content: unknown;
  flag?: string;
  points?: number;
  order_index?: number;
  is_published?: boolean;
}

export interface LabProgress {
  completed: boolean;
  best_score: number;
  total_attempts: number;
  completed_at: string | null;
}

export interface SubmissionResult {
  is_correct: boolean;
  score: number;
  max_score: number;
  feedback: string | null;
  question_results: QuestionResult[] | null;
  flag_results: FlagResult[] | null;
}

export interface QuestionResult {
  question_id: string;
  is_correct: boolean;
  points_earned: number;
  correct_answer: string | null;
  explanation: string | null;
}

export interface FlagResult {
  flag_id: string;
  name: string;
  is_correct: boolean;
  points_earned: number;
}

export interface Submission {
  id: string;
  answer: Record<string, unknown> | null;
  is_correct: boolean;
  score: number;
  attempts: number;
  submitted_at: string;
}

export interface CourseProgress {
  course_id: string;
  user_id: string;
  total_labs: number;
  completed_labs: number;
  total_points_possible: number;
  total_points_earned: number;
  completion_percentage: number;
  lab_progress: LabProgressSummary[];
}

export interface LabProgressSummary {
  lab_id: string;
  lab_title: string;
  lab_type: 'form' | 'ctf' | 'interactive';
  points: number;
  completed: boolean;
  best_score: number;
  total_attempts: number;
  completed_at: string | null;
}

export interface AdminStats {
  total_users: number;
  total_courses: number;
  total_labs: number;
  total_submissions: number;
  total_enrollments: number;
  success_rate: string;
}

export interface AdminUser extends UserPublic {
  enrolled_courses: number;
  completed_labs: number;
}

export interface UpdateUser {
  username?: string;
  bio?: string;
  avatar_url?: string;
  is_active?: boolean;
  role?: string;
}

export interface CourseMonitoring {
  course_id: string;
  course_title: string;
  total_enrolled: number;
  student_progress: StudentProgress[];
}

export interface StudentProgress {
  user_id: string;
  username: string;
  email: string;
  completed_labs: number;
  total_points: number;
  last_activity: string | null;
}

export interface AdminEnrollment {
  user_id: string;
  username: string;
  email: string;
  enrolled_at: string;
}

export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  is_me: boolean;
  username: string;
  completed_labs: number;
  total_points: number;
  last_activity: string | null;
}

export interface LabStats {
  total_submissions: number;
  unique_students: number;
  success_rate: number;
  avg_score: number;
  max_score_achieved: number;
  completed_count: number;
  avg_attempts_to_complete: number;
}

export interface LabInstance {
  instance_id?: string;
  status: 'running' | 'stopped' | 'none';
  started_at?: string;
  expires_at?: string;
}

// ── Lessons ───────────────────────────────────────────────────────────────────

export type LessonBlock =
  | { id: string; type: 'text'; markdown: string }
  | { id: string; type: 'video'; title: string; url: string; duration?: number }
  | { id: string; type: 'markdown_file'; title?: string; url: string };

// Student view — no content, has viewed
export interface LessonSummary {
  id: string;
  course_id: string;
  title: string;
  order_index: number;
  is_published: boolean;
  viewed: boolean;
  created_at: string;
  updated_at: string;
}

// Admin list view — has content, no viewed
export interface LessonAdminRow {
  id: string;
  course_id: string;
  title: string;
  order_index: number;
  is_published: boolean;
  content: LessonBlock[];
  created_at: string;
  updated_at: string;
}

// Single lesson GET — has content + viewed
export interface LessonDetail extends LessonSummary {
  content: LessonBlock[];
}

export interface CreateLesson {
  title: string;
  order_index?: number;
  is_published?: boolean;
  content?: LessonBlock[];   // omit to preserve existing content (partial update)
}

export const lessonsApi = {
  list: (courseId: string, token: string) =>
    api.get<{ lessons: LessonSummary[] }>(`/courses/${courseId}/lessons`, token),
  get: (courseId: string, lessonId: string, token: string) =>
    api.get<{ lesson: LessonDetail }>(`/courses/${courseId}/lessons/${lessonId}`, token),
  complete: (courseId: string, lessonId: string, token: string) =>
    api.post(`/courses/${courseId}/lessons/${lessonId}/complete`, {}, token),
};

export const adminLessonsApi = {
  list: (courseId: string, token: string) =>
    api.get<{ lessons: LessonAdminRow[] }>(`/courses/${courseId}/lessons`, token),
  get: (courseId: string, lessonId: string, token: string) =>
    api.get<{ lesson: LessonDetail }>(`/courses/${courseId}/lessons/${lessonId}`, token),
  create: (courseId: string, data: CreateLesson, token: string) =>
    api.post<{ lesson: LessonAdminRow }>(`/admin/courses/${courseId}/lessons`, data, token),
  update: (courseId: string, lessonId: string, data: CreateLesson, token: string) =>
    api.put<{ lesson: LessonAdminRow }>(`/admin/courses/${courseId}/lessons/${lessonId}`, data, token),
  delete: (courseId: string, lessonId: string, token: string) =>
    api.delete(`/admin/courses/${courseId}/lessons/${lessonId}`, token),
};

export const uploadsApi = {
  video: async (file: File, token: string): Promise<{ url: string; filename: string; kind: string }> => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch('/api/admin/uploads/video', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: form,
    });
    if (!res.ok) {
      let msg = `HTTP ${res.status}`;
      try { const e = await res.json(); msg = e.error || msg; } catch {}
      throw new ApiError(res.status, msg);
    }
    return res.json();
  },
};

export interface AdminSubmission {
  id: string;
  user_id: string;
  username: string;
  is_correct: boolean;
  score: number;
  attempts: number;
  submitted_at: string;
}
