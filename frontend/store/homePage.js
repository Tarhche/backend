 import {defineStore} from "pinia"

 export const useHomePage = defineStore('homePage ' , {
    state () {
        return {
            data: null,
        }
    },
     getters: {
        getData(state) {
            return state.data
        }
    },
    actions: {
        async fetchData() {
            const data = await $fetch(useApiUrlResolver().resolve("api/home"))
            this.data = data
        }
    }
})