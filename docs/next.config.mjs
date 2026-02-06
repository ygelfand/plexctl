import nextra from 'nextra'

const withNextra = nextra({
  // Nextra configuration options
})

export default withNextra({
  output: 'export',
  images: {
    unoptimized: true
  }
})
