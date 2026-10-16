// One function per API endpoint, so pages never build URLs by hand.
import { api } from './client'
import type {
  AdminShow, AppConfig, Booking, Hold, Movie, MovieInput, Seat, SeatMapResponse, Show, User,
} from '../types'

export type DemoAccountKey = 'admin' | 'customer1' | 'customer2' | 'customer3'

export const authApi = {
  me: () => api.get<{ user: User | null }>('/auth/me'),
  login: (email: string, password: string) => api.post<{ user: User }>('/auth/login', { email, password }),
  signup: (name: string, email: string, password: string) =>
    api.post<{ user: User }>('/auth/signup', { name, email, password }),
  // Sends only the allowlisted account key: no credentials live in the frontend.
  demoLogin: (account: DemoAccountKey) => api.post<{ user: User }>('/auth/demo-login', { account }),
  logout: () => api.post<void>('/auth/logout'),
}

export const configApi = {
  get: () => api.get<AppConfig>('/config'),
}

export const moviesApi = {
  list: () => api.get<{ movies: Movie[] }>('/movies'),
  get: (id: number) => api.get<{ movie: Movie; shows: Show[] }>(`/movies/${id}`),
}

export const seatsApi = {
  seatMap: (showId: number) => api.get<SeatMapResponse>(`/shows/${showId}/seats`),
  hold: (showId: number, seatIds: number[]) =>
    api.post<{ hold: Hold; serverTime: string }>(`/shows/${showId}/holds`, { seatIds }),
  getHold: (holdId: string) => api.get<{ hold: Hold; serverTime: string }>(`/holds/${holdId}`),
  releaseHold: (holdId: string) => api.del(`/holds/${holdId}`),
  pay: (holdId: string, outcome: 'success' | 'failure') =>
    api.post<{ booking: Booking; alreadyConfirmed: boolean }>(`/holds/${holdId}/payment`, { outcome }),
}

export const bookingsApi = {
  list: () => api.get<{ bookings: Booking[] }>('/bookings'),
  get: (id: number) => api.get<{ booking: Booking }>(`/bookings/${id}`),
  cancel: (id: number) => api.post<{ booking: Booking }>(`/bookings/${id}/cancel`),
}

export const adminApi = {
  createMovie: (input: MovieInput) => api.post<{ movie: Movie }>('/admin/movies', input),
  updateMovie: (id: number, input: MovieInput) => api.put<{ movie: Movie }>(`/admin/movies/${id}`, input),
  deleteMovie: (id: number) => api.del(`/admin/movies/${id}`),
  listShows: () => api.get<{ shows: AdminShow[] }>('/admin/shows'),
  getShow: (id: number) => api.get<{ show: AdminShow; bookings: Booking[]; seats: Seat[] }>(`/admin/shows/${id}`),
  createShow: (input: { movieId: number; date: string; startTime: string; pricePaise: number }) =>
    api.post<{ show: AdminShow }>('/admin/shows', input),
  deleteShow: (id: number) => api.del(`/admin/shows/${id}`),
}
