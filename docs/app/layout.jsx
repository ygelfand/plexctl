import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'

export const metadata = {
  title: {
    template: '%s – plexctl',
    default: 'plexctl'
  },
  description: 'A robust Plex CLI and TUI written in Go',
  openGraph: {
    title: 'plexctl',
    description: 'A robust Plex CLI and TUI written in Go'
  },
  icons: {
    icon: '/logo.png'
  }
}

export const viewport = {
  width: 'device-width',
  initialScale: 1.0
}

const navbar = (
  <Navbar
    logo={
      <>
        <img src="/logo.png" alt="plexctl" style={{ height: '32px', marginRight: '10px' }} />
        <span style={{ fontWeight: 800 }}>plexctl</span>
      </>
    }
    projectLink="https://github.com/ygelfand/plexctl"
  />
)

const footer = (
  <Footer>
    plexctl Documentation © {new Date().getFullYear()}
  </Footer>
)

export default async function RootLayout({ children }) {
  const pageMap = await getPageMap()
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head />
      <body>
        <Layout
          navbar={navbar}
          footer={footer}
          editLink="Edit this page on GitHub"
          docsRepositoryBase="https://github.com/ygelfand/plexctl/tree/main/docs"
          sidebar={{ defaultMenuCollapseLevel: 1 }}
          pageMap={pageMap}
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
