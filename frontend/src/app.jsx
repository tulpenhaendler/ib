import { useState, useEffect } from 'preact/hooks'
import Router from 'preact-router'
import { Link } from 'preact-router/match'
import { fetchConfig } from './api'
import { List } from './pages/List'
import { Detail } from './pages/Detail'

export function App() {
  const [config, setConfig] = useState({ title: 'ib Backup' })

  useEffect(() => {
    fetchConfig().then(setConfig)
  }, [])

  useEffect(() => {
    document.title = config.title
  }, [config.title])

  return (
    <>
      <header>
        <div class="inner">
          <h1>
            <Link href="/">{config.title}</Link>
          </h1>
        </div>
      </header>
      <div class="container">
        <Router>
          <List path="/" />
          <Detail path="/backup/:id" />
        </Router>
      </div>
    </>
  )
}
