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
  get:    <T>(path: string, token?: string) => request<T>('GET',    path, undefined, token),
  post:   <T>(path: string, body: unknown, token?: string) => request<T>('POST',   path, body, token),
  put:    <T>(path: string, body: unknown, token?: string) => request<T>('PUT',    path, body, token),
  delete: <T>(path: string, token?: string) => request<T>('DELETE', path, undefined, token),
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
  role: 'admin' | 'student';
  is_active: boolean;
  avatar_url: string | null;
  bio: string | null;
  auth_provider: string;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: UserPublic;
}

export interface Course {
  id: string;
  title: string;
  description: string | null;
  thumbnail: string | null;
  category: string | null;
  difficulty: string | null;
  is_published: boolean;
  created_by: string;
  creator_username: string | null;
  lab_count: number;
  enrollment_count: number;
  created_at: string;
  updated_at: string;
}

export interface MyCourse extends Course {
  completed_labs: number;
  total_score: number;
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

export interface PublicSettings {
  registration_enabled: string;
  sso_local_login_enabled: string;
  password_min_length: string;
  password_require_uppercase: string;
  password_require_number: string;
}

export interface PlatformSetting {
  key: string;
  value: string;
  description: string | null;
}

export interface CourseEnrollment {
  user_id: string;
  username: string;
  email: string;
  enrolled_at: string;
}

export interface AdminStats {
  total_users: number;
  total_courses: number;
  total_labs: number;
  total_submissions: number;
  total_enrollments: number;
  success_rate: string;
}

export interface Lab {
  id: string;
  course_id: string;
  title: string;
  description: string | null;
  lab_type: 'form' | 'ctf' | 'interactive';
  module_type?: 'video' | 'image' | 'text';
  content: Record<string, unknown>;
  points: number;
  order_index: number;
  is_published: boolean;
  created_at: string;
  updated_at: string;
}

export interface LabProgress {
  completed: boolean;
  best_score: number;
  total_attempts: number;
  completed_at: string | null;
}

export interface LabWithProgress {
  lab: Lab;
  progress: LabProgress | null;
}

export interface CourseProgress {
  course_id: string;
  user_id: string;
  total_labs: number;
  completed_labs: number;
  total_points_possible: number;
  total_points_earned: number;
  completion_percentage: number;
  lab_progress: {
    lab_id: string;
    lab_title: string;
    lab_type: string;
    points: number;
    completed: boolean;
    best_score: number;
    total_attempts: number;
    completed_at: string | null;
  }[];
}

export interface SubmissionResult {
  is_correct: boolean;
  score: number;
  max_score: number;
  feedback: string | null;
  question_results?: {
    question_id: string;
    is_correct: boolean;
    points_earned: number;
    correct_answer: string | null;
    explanation: string | null;
  }[] | null;
  flag_results?: {
    flag_id: string;
    name: string;
    is_correct: boolean;
    points_earned: number;
  }[] | null;
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

export const ssoApi = {
  providers: () => api.get<{ providers: OAuthProvider[] }>('/auth/oauth/providers'),
  authorize: (provider: string) =>
    api.get<{ url: string; state: string }>(`/auth/oauth/${provider}/authorize`),
  callback: (code: string, state: string) =>
    api.post<AuthResponse>('/auth/oauth/callback', { code, state }),
};

export const publicSettingsApi = {
  get: () => api.get<PublicSettings>('/settings/public'),
};

export const adminSettingsApi = {
  get: (token: string) =>
    api.get<{ settings: PlatformSetting[] }>('/admin/settings', token),
  update: (data: Record<string, string>, token: string) =>
    api.put<{ settings: PlatformSetting[] }>('/admin/settings', data, token),
};

// ── Courses ───────────────────────────────────────────────────────────────────

export const coursesApi = {
  list: (params?: { category?: string; difficulty?: string; search?: string; page?: number; per_page?: number }) => {
    const qs = params ? '?' + new URLSearchParams(
      Object.fromEntries(Object.entries(params).filter(([_, v]) => v != null).map(([k, v]) => [k, String(v)]))
    ).toString() : '';
    return api.get<{ courses: Course[]; total: number; page: number; per_page: number; total_pages: number }>(`/courses${qs}`);
  },
  get: (id: string) => api.get<Course>(`/courses/${id}`),
  enroll: (id: string, token: string) =>
    api.post(`/courses/${id}/enroll`, {}, token),
  unenroll: (id: string, token: string) =>
    api.delete(`/courses/${id}/unenroll`, token),
  myCourses: (token: string) =>
    api.get<{ courses: MyCourse[] }>('/my/courses', token),
};

// ── Labs ──────────────────────────────────────────────────────────────────────

export const labsApi = {
  list: (courseId: string, token: string) =>
    api.get<{ labs: Lab[] }>(`/courses/${courseId}/labs`, token),
  get: (courseId: string, labId: string, token: string) =>
    api.get<LabWithProgress>(`/courses/${courseId}/labs/${labId}`, token),
  submit: (courseId: string, labId: string, answer: unknown, token: string) =>
    api.post<SubmissionResult>(`/courses/${courseId}/labs/${labId}/submit`, { answer }, token),
  progress: (courseId: string, token: string) =>
    api.get<CourseProgress>(`/courses/${courseId}/progress`, token),
};

// ── Admin ─────────────────────────────────────────────────────────────────────

export const adminApi = {
  stats: (token: string) =>
    api.get<AdminStats>('/admin/stats', token),

  users: (token: string) =>
    api.get<{ users: AdminUser[]; total: number }>('/admin/users', token),
  getUser: (id: string, token: string) =>
    api.get<AdminUser>(`/admin/users/${id}`, token),
  updateUser: (id: string, data: UpdateUser, token: string) =>
    api.put(`/admin/users/${id}`, data, token),
  deleteUser: (id: string, token: string) =>
    api.delete(`/admin/users/${id}`, token),

  adminCourses: (token: string, params?: { page?: number; per_page?: number; search?: string }) => {
    const qs = params ? '?' + new URLSearchParams(
      Object.fromEntries(Object.entries(params).filter(([_, v]) => v != null).map(([k, v]) => [k, String(v)]))
    ).toString() : '';
    return api.get<{ courses: Course[]; total: number; page: number; per_page: number }>(`/admin/courses${qs}`, token);
  },

  listEnrollments: (courseId: string, token: string) =>
    api.get<{ enrollments: CourseEnrollment[] }>(`/admin/courses/${courseId}/enrollments`, token),
  enrollUser: (courseId: string, userId: string, token: string) =>
    api.post(`/admin/courses/${courseId}/enrollments`, { user_id: userId }, token),
  unenrollUser: (courseId: string, userId: string, token: string) =>
    api.delete(`/admin/courses/${courseId}/enrollments/${userId}`, token),
};
