import { useState } from 'react'
import Tabs from '../../components/ui/Tabs'
import MenuList from './MenuList'
import SectionList from './SectionList'

const tabs = ['Menus', 'Sections']

// Menus and menu sections are one module: one sidebar entry and one
// permission ("menus") cover both tabs.
function Menus() {
  const [tab, setTab] = useState('Menus')

  return (
    <div className="space-y-4">
      <Tabs tabs={tabs} value={tab} onChange={setTab} />

      {tab === 'Menus' ? <MenuList /> : <SectionList />}
    </div>
  )
}

export default Menus
