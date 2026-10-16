import { Route, Routes } from 'react-router-dom'
import { ProtectedRoute } from './components/ProtectedRoute'
import { Spinner } from './components/Feedback'
import { useAuth } from './hooks/useAuth'
import { MainLayout } from './layouts/MainLayout'
import { BookingConfirmationPage } from './pages/BookingConfirmationPage'
import { CheckoutPage } from './pages/CheckoutPage'
import { HomePage } from './pages/HomePage'
import { LoginPage } from './pages/LoginPage'
import { MovieDetailPage } from './pages/MovieDetailPage'
import { MyBookingsPage } from './pages/MyBookingsPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { SeatSelectionPage } from './pages/SeatSelectionPage'
import { SignupPage } from './pages/SignupPage'
import { AdminDashboardPage } from './pages/admin/AdminDashboardPage'
import { AdminShowDetailPage } from './pages/admin/AdminShowDetailPage'

export default function App() {
  const { loading } = useAuth()
  if (loading) return <Spinner label="Starting Yozigo…" />

  return (
    <Routes>
      <Route element={<MainLayout />}>
        <Route index element={<HomePage />} />
        <Route path="movies/:id" element={<MovieDetailPage />} />
        <Route path="login" element={<LoginPage />} />
        <Route path="signup" element={<SignupPage />} />

        <Route path="shows/:id/seats" element={<ProtectedRoute><SeatSelectionPage /></ProtectedRoute>} />
        <Route path="checkout/:holdId" element={<ProtectedRoute><CheckoutPage /></ProtectedRoute>} />
        <Route path="bookings" element={<ProtectedRoute><MyBookingsPage /></ProtectedRoute>} />
        <Route path="bookings/:id" element={<ProtectedRoute><BookingConfirmationPage /></ProtectedRoute>} />

        <Route path="admin" element={<ProtectedRoute adminOnly><AdminDashboardPage /></ProtectedRoute>} />
        <Route path="admin/shows/:id" element={<ProtectedRoute adminOnly><AdminShowDetailPage /></ProtectedRoute>} />

        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
