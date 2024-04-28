// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
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
