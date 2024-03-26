 import {defineStore} from "pinia"

 export const useTarcheApi =defineStore('tarche ' , {
    state (){
        return {
                homeAll: "",
                homePopular:"",
                homeElements:""
        }
    },
     getters:{
        getHome(state){
            return state.homeAll
        }
     },
    actions: {
        async fetchHomeData() {
            const data = await $fetch( 'https://tarhche-backend.liara.run/api/home')
            this.homeAll = data

        }
    }
})