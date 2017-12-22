module Views.Menu.List exposing (view)

import Html exposing (Html)
import Html.Attributes as Attr
import Html.Events as Event
import Page.MenuModel exposing (Model, Retro)
import Page.MenuMsg exposing (..)


view : Model -> Html Msg
view model =
    Html.div [ Attr.class "menu" ]
        [ Html.ul [ Attr.class "menu-list" ]
            [ Html.li []
                [ Html.a [ Event.onClick NewRetro ]
                    [ Html.text "+ New Retro" ]
                ]
            ]
        , Html.p [ Attr.class "menu-label" ]
            [ Html.text "Your Retros" ]
        , Html.ul [ Attr.class "menu-list" ]
            (List.map (choice model.currentChoice) model.retroList)
        ]


choice : Maybe Retro -> Retro -> Html Msg
choice current { id, name } =
    Html.li []
        [ Html.a
            [ Event.onClick (ShowRetroDetails id)
            , Attr.classList [ ( "is-active", Just id == Maybe.map .id current ) ]
            ]
            [ Html.text name ]
        ]
