<script setup lang="ts">
import Loader from "~/components/loading/loader.vue";

const showModal = ref(false)
const {public: {baseURL}} = useRuntimeConfig()
const cookie = useState("cookie")
const postData = ref("")
const uuid = ref("")
const {
  data: response,
  pending,
  error
} = await useFetch(() => 'https://tarhche-backend.liara.run/api/dashboard/articles', {
  headers: {
    authorization: `Bearer ${cookie.value}`
  },
})

if (error) {
  console.log(error)
}
// async function removePost(uuid){
//  const {data , pending , error , refresh } = await useFetch( ()=>`https://tarhche-backend.liara.run/api/dashboard/articles/${uuid}` , {
//    headers:{
//      authorization:`Bearer ${cookie.value}`
//    }
//  })
//   if (!error){
//     alert("success")
//     await refreshNuxtData()
//   }
//   console.log(error)
//   console.log(uuid)
//   await refreshNuxtData()
//
// }
function changePost(id) {
  showModal.value = true
  uuid.value = id
}

watch(response, () => {
  if (response.value) {
    postData.value = response.value.items
  }
})
async function putData(value){
    const cookie = useCookie("tarche")
    const {status, error}  = await useFetch( "https://tarhche-backend.liara.run/api/dashboard/articles" , {
      method:"put",
      headers:{
        Authorization: `Bearer ${cookie.value}`,
      },
      body:value
    })
    if (status.value == "success"){
      showModal.value = false
      await refreshNuxtData()
    }
    console.log(error.value , "error")
  }
async  function deletePost(id){
  const {status, error}  = await useFetch( `https://tarhche-backend.liara.run/api/dashboard/articles/${id}` , {
    method:"delete",
    headers:{
      Authorization: `Bearer ${cookie.value}`
    }
  })
  if (status.value == "success"){
    alert("آیتم مورد نظر با موفقیت حذف شد ")
    await refreshNuxtData()
  }
  if (error.value){
    console.log(error.value)
  }
}
</script>

<template>
  <div class="articles-list" dir="rtl">
    <div class="loading-container" v-if="pending">
      <loader/>
    </div>
    <div class="header-list d-flex justify-content-between align-items-center mb-2">
      <div class="title-list">لیست اطلاعیه ها</div>
      <div class="create-list px-3 py-1">
        <router-link to="/dashboard/articles/create">ثبت اطلاعیه جدید</router-link>
      </div>
    </div>
    <div class="list-body ">
      <div class="table-responsive-md">
        <table class="table table-striped rounded overflow-hidden align-middle w-100">
          <thead class="table-dark text-center  ">
          <tr class="py-3">
            <th>ردیف</th>
            <th>تصویر</th>
            <th>ناشر</th>
            <th>شناسه</th>
            <th>عنوان</th>
            <th> تاریخ انتشار</th>
            <th>عملیات</th>
          </tr>
          </thead>
          <tbody class="text-center " v-if="postData.length">
          <tr v-for="(item,index) in postData" :key="index">
            <th>{{ index + 1 }}</th>
            <td class=""><img class="img-thumbnail rounded-circle w-50" src="" alt=""></td>
            <td>{{ item.author.name }}</td>
            <td><span class="limited" v-if="item.uuid">{{ item.uuid }}</span></td>
            <td class="list-header" v-if="item.title"><span class="limited">{{ item.title }}</span></td>
            <td><span class="limited" v-if="item.published_at">{{ item.published_at }}</span></td>
            <td class="action ">
              <span class="mx-md-2 d-block d-md-inline-block " @click="deletePost(item.uuid)"><i class="fa-solid fa-trash text-danger"></i></span>
              <span class="mx-md-2 d-block d-md-inline-block " @click="changePost(item.uuid)"><i class="fa-solid fa-pen text-warning"></i></span>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="row">
      <section>
        <transition name="transition">
          <div class="show-modal d-flex justify-content-center overflow-scroll" v-if="showModal"
               @click.self="showModal=false">
            <div class="inner-modal" v-if="response">
              <dashboard-articles-create :data="uuid" v-if="showModal" @send="putData"/>
            </div>
          </div>
        </transition>
      </section>
    </div>
  </div>
</template>

<style scoped>
thead tr {
  color: #817d7d;
  font-size: 0.9rem;
}

tbody tr {
  color: #817d7d;
  font-size: 0.8rem;
}

.list-header {
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}


.limited {
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  line-clamp: 1;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  height: 100%;
}

.fa-trash {
  cursor: pointer;
}

.show-modal {
  position: fixed;
  width: 100%;
  height: 100%;
  background-color: rgba(232, 232, 232, 0.82);
  inset: 0px;
  backdrop-filter: blur(10px);
  z-index: 10;
}

.inner-modal {
  margin-top: 10vh;
  margin-bottom: 5vh !important;
}

.transition-enter-active {
  transition: all 0.7s ease;

}

.transition-leave-active {
  transition: all 0.5s ease;

}


.transition-enter-from, .transition-leave-to {
  opacity: 0;
  transform: translatey(-100%);
}

.transition-enter-to, .transition-leave-from {
  opacity: 1;
}
</style>