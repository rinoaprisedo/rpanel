import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import Navbar from './Navbar'
import Footer from './Footer'
import PanelSslBanner from './PanelSslBanner'

function AppLayout() {
  return (
    <div className="min-h-screen bg-surface-bg">
      <Sidebar />
      <div className="flex min-h-screen flex-col lg:pl-[240px]">
        <Navbar />
        <PanelSslBanner />
        <main className="flex-1 p-4 pb-16 md:p-[28px] md:pb-14">
          <Outlet />
        </main>
        <Footer />
      </div>
    </div>
  )
}

export default AppLayout
