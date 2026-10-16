// Shapes of the JSON returned by the Go API.

export type Role = 'customer' | 'admin'

export interface User {
  id: number
  name: string
  email: string
  role: Role
  createdAt: string
}

export interface AppConfig {
  demoLoginEnabled: boolean
  timezone: string
  holdDurationSeconds: number
  maxSeatsPerBooking: number
}

export type Rating = 'U' | 'UA' | 'A'

export interface Movie {
  id: number
  title: string
  description: string
  language: string
  durationMinutes: number
  rating: Rating
  posterUrl: string | null
  createdAt: string
  updatedAt: string
}

export interface Show {
  id: number
  movieId: number
  movieTitle: string
  movieLanguage: string
  movieRating: Rating
  durationMinutes: number
  startTime: string
  endTime: string
  pricePaise: number
  totalSeats: number
  bookedSeats: number
  heldSeats: number
  remainingSeats: number
}

export interface AdminShow extends Show {
  occupancyPercent: number
  revenuePaise: number
  bookingCount: number
}

export type SeatStatus = 'available' | 'held' | 'held_by_you' | 'booked'

export interface Seat {
  id: number
  label: string
  row: string
  number: number
  status: SeatStatus
}

export interface SeatMapResponse {
  show: Show
  seats: Seat[]
  activeHoldId: string | null
  maxSeats: number
  serverTime: string
}

export type HoldStatus = 'ACTIVE' | 'EXPIRED' | 'CONFIRMED' | 'RELEASED'

export interface Hold {
  id: string
  showId: number
  status: HoldStatus
  expiresAt: string
  seats: { id: number; label: string }[]
  pricePaise: number
  totalPaise: number
  bookingId: number | null
  show: Show
}

export interface Booking {
  id: number
  showId: number
  movieId: number
  movieTitle: string
  startTime: string
  endTime: string
  seats: string[]
  totalPaise: number
  status: 'CONFIRMED' | 'CANCELLED'
  createdAt: string
  cancelledAt: string | null
  cancellable: boolean
  customerName?: string
  customerEmail?: string
}

export interface MovieInput {
  title: string
  description: string
  language: string
  durationMinutes: number
  rating: Rating
  posterUrl: string | null
}
