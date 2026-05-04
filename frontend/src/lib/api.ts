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

// Course — content is file-based, identified by slug
export interface Course {
  slug: string;
  title: string;
  description: string;
  category: string;
  difficulty: 'beginner' | 'intermediate' | 'advanced' | '';
  is_published: boolean;
  lesson_count: number;
  source?: string; // "local" or git repo URL
}

export interface MyCourse extends Course {
  viewed_lessons: number;
  last_activity: string | null;
}

export interface CourseFilter {
  category?: string;
  difficulty?: string;
  search?: string;
}

// Lesson — simple Markdown content
export interface LessonSummary {
  slug: string;
  title: string;
  order: number;
  viewed: boolean;
}

export interface LessonDetail extends LessonSummary {
  content: string; // raw Markdown
}

// Git repository
export interface GitRepo {
  id: string;
  url: string;
  branch: string;
  has_token: boolean;
  status: 'pending' | 'syncing' | 'synced' | 'error';
  error_message: string | null;
  last_synced_at: string | null;
  created_at: string;
}

// Admin
export interface AdminStats {
  total_users: number;
  total_courses: number;
  total_enrollments: number;
}

export interface AdminUser extends UserPublic {
  enrolled_courses: number;
  viewed_lessons: number;
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
  list: (params?: CourseFilter) => {
    const qs = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
    return api.get<{ courses: Course[]; total: number }>(`/courses${qs}`);
  },
  get: (slug: string) => api.get<Course>(`/courses/${slug}`),
  enroll: (slug: string, token: string) =>
    api.post(`/courses/${slug}/enroll`, {}, token),
  unenroll: (slug: string, token: string) =>
    api.delete(`/courses/${slug}/unenroll`, token),
  myCourses: (token: string) =>
    api.get<{ courses: MyCourse[] }>('/my/courses', token),
};

// ── Lessons ───────────────────────────────────────────────────────────────────

export const lessonsApi = {
  list: (courseSlug: string, token: string) =>
    api.get<{ lessons: LessonSummary[] }>(`/courses/${courseSlug}/lessons`, token),
  get: (courseSlug: string, lessonSlug: string, token: string) =>
    api.get<{ lesson: LessonDetail }>(`/courses/${courseSlug}/lessons/${lessonSlug}`, token),
  complete: (courseSlug: string, lessonSlug: string, token: string) =>
    api.post(`/courses/${courseSlug}/lessons/${lessonSlug}/complete`, {}, token),
};

// ── Git repos ─────────────────────────────────────────────────────────────────

export const reposApi = {
  list: (token: string) =>
    api.get<{ repos: GitRepo[] }>('/my/repos', token),
  add: (data: { url: string; branch: string; token?: string }, token: string) =>
    api.post<GitRepo>('/my/repos', data, token),
  remove: (id: string, token: string) =>
    api.delete(`/my/repos/${id}`, token),
  sync: (id: string, token: string) =>
    api.post<{ message: string; last_synced_at: string }>(`/my/repos/${id}/sync`, {}, token),
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
  courses: (token: string) =>
    api.get<{ courses: Course[]; total: number }>('/admin/courses', token),
};
