// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  build: {
    
  },
  vite: {
    build: {
      cssCodeSplit: false,
   },
  },
  webpack: {
    extractCSS: true,
    optimization: {
      splitChunks: {
        cacheGroups: {
          styles: {
            name: 'styles',
            test: /\.(css|vue)$/,
            chunks: 'all',
            enforce: true
          }
        }
      }
    }
  },
  modules: ['@vueuse/nuxt', "nuxt-tiptap-editor"],
  runtimeConfig: {
    internalApiBaseUrl: '',
    public: {
      apiBaseUrl: '',
    }
  },
  app:{
    head:{
      htmlAttrs:{
        dir: "rtl",
        lang: "fa",
      }
    }
  }
})
