<script setup>
const props = defineProps(['currentPage', 'totalPage', 'hideNext', 'hidePrev', 'nextIcon', 'prevIcon'])
const {currentPage, totalPage, hideNext, hidePrev} = props
const button = ref(null)
const page = ref(Number(currentPage))
const emit = defineEmits(['changePage'])

function activePage(index) {
  if (index === "+") {
    if (page.value < Number(totalPage)){
    page.value++
      removeActive()
      button.value[page.value-1].classList.add('active')
      emit('changePage', page.value)
    }
  }
  else if (index === "-") {
    if (page.value > 1){
      page.value--
      removeActive()
      button.value[page.value-1].classList.add('active')
      emit('changePage', page.value)
    }
  }
  else {
    page.value = index + 1
    removeActive()
    button.value[index].classList.add('active')
    emit('changePage', page.value)
  }
}
function removeActive(){
  button.value.forEach((item) => {
    item.classList.remove('active')
  })
}
</script>

<template>
  <div class="pagination">
    <ul class="d-flex list-unstyled flex-row-reverse ">
      <li class="prev-icon  rounded-start overflow-hidden" @click="activePage('-')" v-if="!hidePrev">
        <button class="btn border-0 rounded-0">{{ prevIcon || '>' }}</button>
      </li>
      <li v-for="(item , index) in Number(totalPage)" :key="index" :class="{active:page-1==index }"
          @click="activePage(index)">
        <button class="btn border-0 rounded-0" ref="button">{{ item }}</button>
      </li>
      <li class="next-icon  rounded-end overflow-hidden" v-if="!hideNext" @click="activePage('+')">
        <button class="btn border-0 rounded-0">{{ nextIcon || '<' }}</button>
      </li>
    </ul>
  </div>
</template>

<style scoped lang="scss">
ul {

  li {
    position: relative;
    overflow: hidden;
    &:last-child {
      border-bottom-right-radius: 3px;
      border-top-right-radius: 3px;
    }

    &:first-child {
      border-bottom-left-radius: 3px;
      border-top-left-radius: 3px;
    }

    &:nth-child(odd) {
      &:after {
        content: "";
        position: absolute;
        right: 0;
        left: 0;
        transition: 200ms;
        bottom: 0;
        width: 100%;
        height: 0;
        background: black;
        z-index: -1;
      }
    }

    &:nth-child(even) {
      &:after {
        content: "";
        position: absolute;
        right: 0;
        left: 0;
        transition: 200ms;
        top: 0;
        width: 100%;
        height: 0;
        background: black;
        z-index: -1;
      }
    }

    &:hover:after {
      height: 100%;
    }

    &:hover {
      color: white;
    }
  }

  .active {
    background: black;

    & button {
      color: white;
    }
  }

}
</style>